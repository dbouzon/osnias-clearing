// SPDX-License-Identifier: MIT
pragma solidity ^0.8.22;

import {ERC20} from "@openzeppelin/contracts/token/ERC20/ERC20.sol";
import {Ownable} from "@openzeppelin/contracts/access/Ownable.sol";
import {Ownable2Step} from "@openzeppelin/contracts/access/Ownable2Step.sol";

/**
 * @title OsniasClearing — Osnias USD Clearing (SEI MAINNET — v1)
 *
 * @notice Osnias USD Clearing — Jeton de clearing P2P avec :
 *  - mint / burn manuel réservé au gestionnaire ;
 *  - demandes de mint / burn émises par un ou plusieurs oracles autorisés ;
 *  - demandes oracle valables 24 heures ;
 *  - exécution finale de toute demande oracle réservée au gestionnaire ;
 *  - circulation P2P libre entre EOA ;
 *  - 6 décimales techniques (l'interface peut n'en afficher que 2) ;
 *  - transfert P2P minimal de 0,50 ORUSD ;
 *  - frais fixes de protocole de 0,15 % déduits du montant reçu par le destinataire (mécanique VISA) ;
 *  - approve() / transferFrom() interdits ;
 *  - transferts vers contrats interdits.
 *
 * IMPORTANT :
 *  L'oracle ne mint et ne burn jamais directement.
 *  Il observe / vérifie une situation externe puis dépose une demande on-chain.
 *  Seul le gestionnaire ORUSD peut ensuite exécuter cette demande.
 *
 * POLYGON MAINNET — adresses initiales intégrées au contrat.
 * Ne pas utiliser en production sans audit ni remplacement des paramètres de test.
 */
contract OsniasClearing is ERC20, Ownable2Step {

    // ─────────────────────────────────────────────────────────────
    // IDENTITÉ DU PROTOCOLE
    // ─────────────────────────────────────────────────────────────

    string public constant PROTOCOL         = "Osnias Clearing";
    string public constant PROTOCOL_VERSION = "1.0";
    string public constant TOKEN_FULL_NAME  = "Osnias USD Clearing";

    // ─────────────────────────────────────────────────────────────
    // FRAIS DU PROTOCOLE
    // ─────────────────────────────────────────────────────────────

    uint256 public constant FEE_BPS = 15; // 0,15 %
    uint256 public constant BPS_DENOMINATOR = 10_000;

    /// @notice Précision technique du token : 6 décimales.
    uint8 public constant ORUSD_DECIMALS = 6;

    /// @notice Montant P2P minimal brut (avant frais) garantissant que le destinataire
    ///         reçoit au moins 0,50 ORUSD après déduction des 0,15 %.
    ///         0,50 ORUSD / (1 - 0,0015) ≈ 500 751 unités — arrondi à 501 000 pour lisibilité.
    uint256 public constant MIN_TRANSFER = 501_000;

    /// @notice Durée de validité d'une demande oracle de test : 24 heures.
    uint256 public constant REQUEST_VALIDITY = 24 hours;

    /// @notice Wallet EOA recevant les frais de protocole.
    address public feeRecipient;

    /// @notice Wallet GasPool — stock de POL natif pour sponsoring gas users.
    ///         Ne peut pas recevoir d'ORUSD.
    address public constant GAS_POOL =
        0x29a1C3F326850502d2629eb117b2451f73A9F345;

    /// @notice Adresses interdites de recevoir des ORUSD.
    ///         Gestionnaire, GasPool et oracles autorisés sont blacklistés.
    mapping(address => bool) public orusdBlacklisted;

    // ─────────────────────────────────────────────────────────────
    // ORACLES
    // ─────────────────────────────────────────────────────────────

    /// @notice Oracles autorisés à déposer des demandes.
    mapping(address => bool) public isOracle;

    /// @notice Nature de la demande oracle.
    enum RequestType {
        MINT,
        BURN
    }

    /// @notice Etat d'une demande oracle.
    enum RequestStatus {
        NONE,
        PENDING,
        EXECUTED,
        REJECTED
    }

    /**
     * @notice Demande déposée par un oracle après contrôle externe.
     *
     * externalRef :
     *   identifiant libre représentant la preuve / escrow / transaction
     *   sur la blockchain source. Pour un test, on peut utiliser keccak256(...)
     *   d'un identifiant externe.
     *
     * sourceChain :
     *   identifiant lisible compact de la blockchain source, encodé en bytes32
     *   (ex. bytes32("ETHEREUM"), bytes32("CRONOS"), bytes32("BITCOIN")).
     */
    struct OracleRequest {
        RequestType requestType;
        RequestStatus status;
        address oracle;
        address account;
        uint256 amount;
        bytes32 sourceChain;
        bytes32 externalRef;
        uint256 createdAt;
    }

    /// @notice requestId => demande.
    mapping(bytes32 => OracleRequest) public oracleRequests;

    /// @dev Empêche qu'une même preuve externe soit utilisée deux fois.
    mapping(bytes32 => bool) public usedExternalProof;

    /// @dev Compteur séquentiel de demandes — garantit l'unicité des requestId.
    uint256 public requestNonce;

    // ─────────────────────────────────────────────────────────────
    // EVENTS
    // ─────────────────────────────────────────────────────────────

    event FeeRecipientChanged(
        address indexed previousRecipient,
        address indexed newRecipient
    );

    event OracleSet(
        address indexed oracle,
        bool allowed
    );

    event OracleRequestCreated(
        bytes32 indexed requestId,
        address indexed oracle,
        RequestType requestType,
        address indexed account,
        uint256 amount,
        bytes32 sourceChain,
        bytes32 externalRef,
        uint256 nonce
    );

    event OracleRequestExecuted(
        bytes32 indexed requestId,
        RequestType requestType,
        address indexed account,
        uint256 amount
    );

    event OracleRequestRejected(
        bytes32 indexed requestId
    );

    event ManualMint(
        address indexed manager,
        address indexed to,
        uint256 amount
    );

    event ManualBurn(
        address indexed manager,
        address indexed from,
        uint256 amount
    );

    // ─────────────────────────────────────────────────────────────
    // ERRORS
    // ─────────────────────────────────────────────────────────────

    error ZeroAddressForbidden();
    error ContractAddressForbidden();
    error ApprovalForbidden();
    error OrusdTransferForbidden(address forbidden);
    error TransferFromForbidden();
    error TransferToContractForbidden();
    error NotAuthorizedOracle();
    error InvalidAmount();
    error TransferBelowMinimum(uint256 amount, uint256 minimum);
    error RequestAlreadyExists();
    error ExternalProofAlreadyUsed();
    error RequestNotPending();
    error RequestExpired(uint256 createdAt, uint256 expiresAt);
    error InsufficientBalanceForAmountAndFee(
        uint256 balance,
        uint256 required
    );
    error InsufficientBalanceForBurn(
        address account,
        uint256 balance,
        uint256 required
    );

    // ─────────────────────────────────────────────────────────────
    // CONSTRUCTOR
    // ─────────────────────────────────────────────────────────────

    /// @notice Gestionnaire initial ORUSD sur Polygon Amoy.
    address public constant INITIAL_MANAGER =
        0xEAcc9B9e1f18AaE5879e9259684578903d09C4CC;

    /// @notice Wallet de collecte des commissions de 0,15 % sur Polygon Amoy.
    address public constant INITIAL_FEE_RECIPIENT =
        0xAD7761445C9CdCC80DFB549E02279353156b5059;

    /**
     * @notice Déploiement de la version de test Polygon Amoy.
     * @dev Les adresses initiales sont figées dans ce fichier.
     *      Le gestionnaire pourra ensuite transférer l'ownership via Ownable2Step,
     *      et pourra modifier feeRecipient via setFeeRecipient().
     */
    constructor()
        ERC20("Osnias Clearing", "ORUSD")
        Ownable(INITIAL_MANAGER)
    {
        if (INITIAL_FEE_RECIPIENT.code.length > 0) {
            revert ContractAddressForbidden();
        }

        feeRecipient = INITIAL_FEE_RECIPIENT;

        // Blacklist ORUSD : Gestionnaire et GasPool ne peuvent pas recevoir d'ORUSD.
        // Le feeRecipient N'EST PAS blacklisté — il doit recevoir les frais 0,15 %.
        orusdBlacklisted[INITIAL_MANAGER] = true;
        orusdBlacklisted[GAS_POOL]        = true;
    }

    /**
     * @notice ORUSD utilise 6 décimales on-chain.
     * @dev L'interface utilisateur pourra choisir d'en afficher seulement 2.
     */
    function decimals() public pure override returns (uint8) {
        return ORUSD_DECIMALS;
    }

    // ─────────────────────────────────────────────────────────────
    // GESTIONNAIRE — MINT / BURN MANUEL
    // ─────────────────────────────────────────────────────────────

    /**
     * @notice Mint manuel de test.
     * @dev Seul le gestionnaire ORUSD peut l'exécuter.
     */
    function mint(address to, uint256 amount) external onlyOwner {
        if (to == address(0)) revert ZeroAddressForbidden();
        if (to.code.length > 0) revert ContractAddressForbidden();
        if (amount == 0) revert InvalidAmount();

        _mint(to, amount);
        emit ManualMint(_msgSender(), to, amount);
    }

    /**
     * @notice Burn manuel de test.
     * @dev Seul le gestionnaire ORUSD peut l'exécuter.
     */
    function burn(address from, uint256 amount) external onlyOwner {
        if (from == address(0)) revert ZeroAddressForbidden();
        if (amount == 0) revert InvalidAmount();

        _burn(from, amount);
        emit ManualBurn(_msgSender(), from, amount);
    }

    // ─────────────────────────────────────────────────────────────
    // GESTION DU WALLET DE FRAIS
    // ─────────────────────────────────────────────────────────────

    function setFeeRecipient(address newRecipient) external onlyOwner {
        if (newRecipient == address(0)) {
            revert ZeroAddressForbidden();
        }

        if (newRecipient.code.length > 0) {
            revert ContractAddressForbidden();
        }

        address previousRecipient = feeRecipient;
        feeRecipient = newRecipient;

        emit FeeRecipientChanged(previousRecipient, newRecipient);
    }

    // ─────────────────────────────────────────────────────────────
    // GESTION DES ORACLES
    // ─────────────────────────────────────────────────────────────

    /**
     * @notice Autorise ou retire un oracle.
     * @dev L'oracle autorisé est automatiquement blacklisté pour réception ORUSD.
     *      La blacklist est maintenue même si l'oracle est retiré — sécurité.
     */
    function setOracle(address oracle, bool allowed) external onlyOwner {
        if (oracle == address(0)) revert ZeroAddressForbidden();

        isOracle[oracle] = allowed;

        // Blacklist ORUSD automatique à l'autorisation.
        if (allowed) {
            orusdBlacklisted[oracle] = true;
        }

        emit OracleSet(oracle, allowed);
    }

    /**
     * @notice L'oracle dépose une demande après vérification de l'escrow externe.
     *
     * @param requestType MINT ou BURN.
     * @param account Wallet Polygon concerné.
     * @param amount Montant ORUSD en unités 6 décimales.
     * @param sourceChain Blockchain observée.
     * @param externalRef Référence unique de la preuve externe.
     *
     * @return requestId Identifiant unique de la demande.
     */
    function requestFromOracle(
        RequestType requestType,
        address account,
        uint256 amount,
        bytes32 sourceChain,
        bytes32 externalRef
    )
        external
        returns (bytes32 requestId)
    {
        if (!isOracle[_msgSender()]) revert NotAuthorizedOracle();
        if (account == address(0)) revert ZeroAddressForbidden();
        if (account.code.length > 0) revert ContractAddressForbidden();
        if (amount == 0) revert InvalidAmount();

        // Une preuve externe donnée ne peut servir qu'à une seule demande.
        bytes32 proofKey = keccak256(
            abi.encode(sourceChain, externalRef)
        );

        if (usedExternalProof[proofKey]) {
            revert ExternalProofAlreadyUsed();
        }

        usedExternalProof[proofKey] = true;

        uint256 nonce = requestNonce++;
        requestId = keccak256(
            abi.encode(
                _msgSender(),
                requestType,
                account,
                amount,
                sourceChain,
                externalRef,
                nonce
            )
        );

        oracleRequests[requestId] = OracleRequest({
            requestType: requestType,
            status: RequestStatus.PENDING,
            oracle: _msgSender(),
            account: account,
            amount: amount,
            sourceChain: sourceChain,
            externalRef: externalRef,
            createdAt: block.timestamp
        });

        emit OracleRequestCreated(
            requestId,
            _msgSender(),
            requestType,
            account,
            amount,
            sourceChain,
            externalRef,
            nonce
        );
    }

    /**
     * @notice Exécute une demande oracle déjà déposée.
     * @dev SEUL le gestionnaire peut appeler cette fonction.
     *      L'oracle n'a donc aucun pouvoir direct de mint / burn.
     */
    function executeOracleRequest(bytes32 requestId)
        external
        onlyOwner
    {
        OracleRequest storage request = oracleRequests[requestId];

        if (request.status != RequestStatus.PENDING) {
            revert RequestNotPending();
        }

        uint256 expiresAt = request.createdAt + REQUEST_VALIDITY;
        if (block.timestamp > expiresAt) {
            revert RequestExpired(request.createdAt, expiresAt);
        }

        // Effets avant interaction interne, pour rendre l'état non rejouable.
        request.status = RequestStatus.EXECUTED;

        if (request.requestType == RequestType.MINT) {
            _mint(request.account, request.amount);
        } else {
            uint256 accountBalance = balanceOf(request.account);
            if (accountBalance < request.amount) {
                revert InsufficientBalanceForBurn(
                    request.account,
                    accountBalance,
                    request.amount
                );
            }
            _burn(request.account, request.amount);
        }

        emit OracleRequestExecuted(
            requestId,
            request.requestType,
            request.account,
            request.amount
        );
    }

    /**
     * @notice Refuse définitivement une demande oracle en attente.
     * @dev La preuve externe reste marquée comme utilisée afin d'éviter
     *      qu'elle soit resoumise sous une autre demande.
     */
    function rejectOracleRequest(bytes32 requestId)
        external
        onlyOwner
    {
        OracleRequest storage request = oracleRequests[requestId];

        if (request.status != RequestStatus.PENDING) {
            revert RequestNotPending();
        }

        request.status = RequestStatus.REJECTED;
        emit OracleRequestRejected(requestId);
    }

    // ─────────────────────────────────────────────────────────────
    // P2P — TRANSFERT LIBRE — MÉCANIQUE VISA (0,15 % DÉDUIT)
    // ─────────────────────────────────────────────────────────────

    /**
     * @notice Transfert P2P — mécanique identique aux réseaux de paiement
     *         (VISA, Mastercard) : les frais de protocole sont déduits du
     *         montant transféré. L'émetteur est débité de `value` exactement.
     *         Le destinataire reçoit `value - fee`.
     *
     * @dev  fee     = value × 0,15 %
     *       reçu    = value - fee = value × 99,85 %
     *       débit   = value (inchangé pour l'émetteur)
     *
     * Exemple :
     *  value envoyée       : 1 000,000000 ORUSD
     *  frais protocole     :     1,500000 ORUSD  (0,15 %)
     *  destinataire reçoit :   998,500000 ORUSD  (99,85 %)
     *  débit émetteur      : 1 000,000000 ORUSD
     *
     * Le montant minimal garanti au destinataire est 0,50 ORUSD,
     * ce qui implique un `value` minimal de ≈ 501 000 unités.
     *
     * Le wallet feeRecipient est exonéré lorsqu'il déplace les frais collectés.
     */
    function transfer(address to, uint256 value)
        public
        override
        returns (bool)
    {
        address from = _msgSender();

        if (from == feeRecipient) {
            _transfer(from, to, value);
            return true;
        }

        if (value < MIN_TRANSFER) {
            revert TransferBelowMinimum(value, MIN_TRANSFER);
        }

        uint256 senderBalance = balanceOf(from);
        if (senderBalance < value) {
            revert InsufficientBalanceForAmountAndFee(
                senderBalance,
                value
            );
        }

        uint256 fee     = (value * FEE_BPS) / BPS_DENOMINATOR;
        uint256 netAmount = value - fee;

        // Le destinataire reçoit value - fee (99,85 % du montant).
        _transfer(from, to, netAmount);

        // Les frais sont prélevés sur le solde restant de l'émetteur.
        if (fee > 0) {
            _transfer(from, feeRecipient, fee);
        }

        return true;
    }

    // ─────────────────────────────────────────────────────────────
    // ANTI-DEX / ANTI-AMM
    // ─────────────────────────────────────────────────────────────

    function approve(address, uint256)
        public
        pure
        override
        returns (bool)
    {
        revert ApprovalForbidden();
    }

    function transferFrom(address, address, uint256)
        public
        pure
        override
        returns (bool)
    {
        revert TransferFromForbidden();
    }

    /**
     * @dev Hook OpenZeppelin v5 :
     *  - mint : autorisé ;
     *  - burn : autorisé ;
     *  - EOA -> EOA : autorisé ;
     *  - transfert vers contrat : interdit.
     */
    function _update(address from, address to, uint256 value)
        internal
        override
    {
        bool minting = (from == address(0));
        bool burning = (to == address(0));

        // Blacklist ORUSD : Gestionnaire, GasPool et oracles ne peuvent pas recevoir.
        if (!burning && orusdBlacklisted[to]) {
            revert OrusdTransferForbidden(to);
        }

        if (!minting && !burning && to.code.length > 0) {
            revert TransferToContractForbidden();
        }

        super._update(from, to, value);
    }
}
