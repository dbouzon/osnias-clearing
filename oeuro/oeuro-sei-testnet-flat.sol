// SPDX-License-Identifier: UNLICENSED
/*
 * Copyright © 2026 Osnias Clearing.
 * All rights reserved.
 *
 * Source code disclosed solely for transparency,
 * audit and on-chain verification purposes.
 *
 * No permission is granted to copy, modify, deploy,
 * distribute, sublicense or commercially exploit
 * this software without prior written authorization.
 */
pragma solidity ^0.8.22;

interface IERC20 {
    /**
     * @dev Emitted when `value` tokens are moved from one account (`from`) to
     * another (`to`).
     *
     * Note that `value` may be zero.
     */
    event Transfer(address indexed from, address indexed to, uint256 value);

    /**
     * @dev Emitted when the allowance of a `spender` for an `owner` is set by
     * a call to {approve}. `value` is the new allowance.
     */
    event Approval(address indexed owner, address indexed spender, uint256 value);

    /**
     * @dev Returns the value of tokens in existence.
     */
    function totalSupply() external view returns (uint256);

    /**
     * @dev Returns the value of tokens owned by `account`.
     */
    function balanceOf(address account) external view returns (uint256);

    /**
     * @dev Moves a `value` amount of tokens from the caller's account to `to`.
     *
     * Returns a boolean value indicating whether the operation succeeded.
     *
     * Emits a {Transfer} event.
     */
    function transfer(address to, uint256 value) external returns (bool);

    /**
     * @dev Returns the remaining number of tokens that `spender` will be
     * allowed to spend on behalf of `owner` through {transferFrom}. This is
     * zero by default.
     *
     * This value changes when {approve} or {transferFrom} are called.
     */
    function allowance(address owner, address spender) external view returns (uint256);

    /**
     * @dev Sets a `value` amount of tokens as the allowance of `spender` over the
     * caller's tokens.
     *
     * Returns a boolean value indicating whether the operation succeeded.
     *
     * IMPORTANT: Beware that changing an allowance with this method brings the risk
     * that someone may use both the old and the new allowance by unfortunate
     * transaction ordering. One possible solution to mitigate this race
     * condition is to first reduce the spender's allowance to 0 and set the
     * desired value afterwards:
     * https://github.com/ethereum/EIPs/issues/20#issuecomment-263524729
     *
     * Emits an {Approval} event.
     */
    function approve(address spender, uint256 value) external returns (bool);

    /**
     * @dev Moves a `value` amount of tokens from `from` to `to` using the
     * allowance mechanism. `value` is then deducted from the caller's
     * allowance.
     *
     * Returns a boolean value indicating whether the operation succeeded.
     *
     * Emits a {Transfer} event.
     */
    function transferFrom(address from, address to, uint256 value) external returns (bool);
}

interface IERC20Metadata is IERC20 {
    /**
     * @dev Returns the name of the token.
     */
    function name() external view returns (string memory);

    /**
     * @dev Returns the symbol of the token.
     */
    function symbol() external view returns (string memory);

    /**
     * @dev Returns the decimals places of the token.
     */
    function decimals() external view returns (uint8);
}

abstract contract Context {
    function _msgSender() internal view virtual returns (address) {
        return msg.sender;
    }

    function _msgData() internal view virtual returns (bytes calldata) {
        return msg.data;
    }

    function _contextSuffixLength() internal view virtual returns (uint256) {
        return 0;
    }
}

interface IERC20Errors {
    /**
     * @dev Indicates an error related to the current `balance` of a `sender`. Used in transfers.
     * @param sender Address whose tokens are being transferred.
     * @param balance Current balance for the interacting account.
     * @param needed Minimum amount required to perform a transfer.
     */
    error ERC20InsufficientBalance(address sender, uint256 balance, uint256 needed);

    /**
     * @dev Indicates a failure with the token `sender`. Used in transfers.
     * @param sender Address whose tokens are being transferred.
     */
    error ERC20InvalidSender(address sender);

    /**
     * @dev Indicates a failure with the token `receiver`. Used in transfers.
     * @param receiver Address to which tokens are being transferred.
     */
    error ERC20InvalidReceiver(address receiver);

    /**
     * @dev Indicates a failure with the `spender`’s `allowance`. Used in transfers.
     * @param spender Address that may be allowed to operate on tokens without being their owner.
     * @param allowance Amount of tokens a `spender` is allowed to operate with.
     * @param needed Minimum amount required to perform a transfer.
     */
    error ERC20InsufficientAllowance(address spender, uint256 allowance, uint256 needed);

    /**
     * @dev Indicates a failure with the `approver` of a token to be approved. Used in approvals.
     * @param approver Address initiating an approval operation.
     */
    error ERC20InvalidApprover(address approver);

    /**
     * @dev Indicates a failure with the `spender` to be approved. Used in approvals.
     * @param spender Address that may be allowed to operate on tokens without being their owner.
     */
    error ERC20InvalidSpender(address spender);
}

abstract contract ERC20 is Context, IERC20, IERC20Metadata, IERC20Errors {
    mapping(address account => uint256) private _balances;

    mapping(address account => mapping(address spender => uint256)) private _allowances;

    uint256 private _totalSupply;

    string private _name;
    string private _symbol;

    /**
     * @dev Sets the values for {name} and {symbol}.
     *
     * All two of these values are immutable: they can only be set once during
     * construction.
     */
    constructor(string memory name_, string memory symbol_) {
        _name = name_;
        _symbol = symbol_;
    }

    /**
     * @dev Returns the name of the token.
     */
    function name() public view virtual returns (string memory) {
        return _name;
    }

    /**
     * @dev Returns the symbol of the token, usually a shorter version of the
     * name.
     */
    function symbol() public view virtual returns (string memory) {
        return _symbol;
    }

    /**
     * @dev Returns the number of decimals used to get its user representation.
     * For example, if `decimals` equals `2`, a balance of `505` tokens should
     * be displayed to a user as `5.05` (`505 / 10 ** 2`).
     *
     * Tokens usually opt for a value of 18, imitating the relationship between
     * Ether and Wei. This is the default value returned by this function, unless
     * it's overridden.
     *
     * NOTE: This information is only used for _display_ purposes: it in
     * no way affects any of the arithmetic of the contract, including
     * {IERC20-balanceOf} and {IERC20-transfer}.
     */
    function decimals() public view virtual returns (uint8) {
        return 18;
    }

    /**
     * @dev See {IERC20-totalSupply}.
     */
    function totalSupply() public view virtual returns (uint256) {
        return _totalSupply;
    }

    /**
     * @dev See {IERC20-balanceOf}.
     */
    function balanceOf(address account) public view virtual returns (uint256) {
        return _balances[account];
    }

    /**
     * @dev See {IERC20-transfer}.
     *
     * Requirements:
     *
     * - `to` cannot be the zero address.
     * - the caller must have a balance of at least `value`.
     */
    function transfer(address to, uint256 value) public virtual returns (bool) {
        address owner = _msgSender();
        _transfer(owner, to, value);
        return true;
    }

    /**
     * @dev See {IERC20-allowance}.
     */
    function allowance(address owner, address spender) public view virtual returns (uint256) {
        return _allowances[owner][spender];
    }

    /**
     * @dev See {IERC20-approve}.
     *
     * NOTE: If `value` is the maximum `uint256`, the allowance is not updated on
     * `transferFrom`. This is semantically equivalent to an infinite approval.
     *
     * Requirements:
     *
     * - `spender` cannot be the zero address.
     */
    function approve(address spender, uint256 value) public virtual returns (bool) {
        address owner = _msgSender();
        _approve(owner, spender, value);
        return true;
    }

    /**
     * @dev See {IERC20-transferFrom}.
     *
     * Emits an {Approval} event indicating the updated allowance. This is not
     * required by the EIP. See the note at the beginning of {ERC20}.
     *
     * NOTE: Does not update the allowance if the current allowance
     * is the maximum `uint256`.
     *
     * Requirements:
     *
     * - `from` and `to` cannot be the zero address.
     * - `from` must have a balance of at least `value`.
     * - the caller must have allowance for ``from``'s tokens of at least
     * `value`.
     */
    function transferFrom(address from, address to, uint256 value) public virtual returns (bool) {
        address spender = _msgSender();
        _spendAllowance(from, spender, value);
        _transfer(from, to, value);
        return true;
    }

    /**
     * @dev Moves a `value` amount of tokens from `from` to `to`.
     *
     * This internal function is equivalent to {transfer}, and can be used to
     * e.g. implement automatic token fees, slashing mechanisms, etc.
     *
     * Emits a {Transfer} event.
     *
     * NOTE: This function is not virtual, {_update} should be overridden instead.
     */
    function _transfer(address from, address to, uint256 value) internal {
        if (from == address(0)) {
            revert ERC20InvalidSender(address(0));
        }
        if (to == address(0)) {
            revert ERC20InvalidReceiver(address(0));
        }
        _update(from, to, value);
    }

    /**
     * @dev Transfers a `value` amount of tokens from `from` to `to`, or alternatively mints (or burns) if `from`
     * (or `to`) is the zero address. All customizations to transfers, mints, and burns should be done by overriding
     * this function.
     *
     * Emits a {Transfer} event.
     */
    function _update(address from, address to, uint256 value) internal virtual {
        if (from == address(0)) {
            // Overflow check required: The rest of the code assumes that totalSupply never overflows
            _totalSupply += value;
        } else {
            uint256 fromBalance = _balances[from];
            if (fromBalance < value) {
                revert ERC20InsufficientBalance(from, fromBalance, value);
            }
            unchecked {
                // Overflow not possible: value <= fromBalance <= totalSupply.
                _balances[from] = fromBalance - value;
            }
        }

        if (to == address(0)) {
            unchecked {
                // Overflow not possible: value <= totalSupply or value <= fromBalance <= totalSupply.
                _totalSupply -= value;
            }
        } else {
            unchecked {
                // Overflow not possible: balance + value is at most totalSupply, which we know fits into a uint256.
                _balances[to] += value;
            }
        }

        emit Transfer(from, to, value);
    }

    /**
     * @dev Creates a `value` amount of tokens and assigns them to `account`, by transferring it from address(0).
     * Relies on the `_update` mechanism
     *
     * Emits a {Transfer} event with `from` set to the zero address.
     *
     * NOTE: This function is not virtual, {_update} should be overridden instead.
     */
    function _mint(address account, uint256 value) internal {
        if (account == address(0)) {
            revert ERC20InvalidReceiver(address(0));
        }
        _update(address(0), account, value);
    }

    /**
     * @dev Destroys a `value` amount of tokens from `account`, lowering the total supply.
     * Relies on the `_update` mechanism.
     *
     * Emits a {Transfer} event with `to` set to the zero address.
     *
     * NOTE: This function is not virtual, {_update} should be overridden instead
     */
    function _burn(address account, uint256 value) internal {
        if (account == address(0)) {
            revert ERC20InvalidSender(address(0));
        }
        _update(account, address(0), value);
    }

    /**
     * @dev Sets `value` as the allowance of `spender` over the `owner` s tokens.
     *
     * This internal function is equivalent to `approve`, and can be used to
     * e.g. set automatic allowances for certain subsystems, etc.
     *
     * Emits an {Approval} event.
     *
     * Requirements:
     *
     * - `owner` cannot be the zero address.
     * - `spender` cannot be the zero address.
     *
     * Overrides to this logic should be done to the variant with an additional `bool emitEvent` argument.
     */
    function _approve(address owner, address spender, uint256 value) internal {
        _approve(owner, spender, value, true);
    }

    /**
     * @dev Variant of {_approve} with an optional flag to enable or disable the {Approval} event.
     *
     * By default (when calling {_approve}) the flag is set to true. On the other hand, approval changes made by
     * `_spendAllowance` during the `transferFrom` operation set the flag to false. This saves gas by not emitting any
     * `Approval` event during `transferFrom` operations.
     *
     * Anyone who wishes to continue emitting `Approval` events on the`transferFrom` operation can force the flag to
     * true using the following override:
     * ```
     * function _approve(address owner, address spender, uint256 value, bool) internal virtual override {
     *     super._approve(owner, spender, value, true);
     * }
     * ```
     *
     * Requirements are the same as {_approve}.
     */
    function _approve(address owner, address spender, uint256 value, bool emitEvent) internal virtual {
        if (owner == address(0)) {
            revert ERC20InvalidApprover(address(0));
        }
        if (spender == address(0)) {
            revert ERC20InvalidSpender(address(0));
        }
        _allowances[owner][spender] = value;
        if (emitEvent) {
            emit Approval(owner, spender, value);
        }
    }

    /**
     * @dev Updates `owner` s allowance for `spender` based on spent `value`.
     *
     * Does not update the allowance value in case of infinite allowance.
     * Revert if not enough allowance is available.
     *
     * Does not emit an {Approval} event.
     */
    function _spendAllowance(address owner, address spender, uint256 value) internal virtual {
        uint256 currentAllowance = allowance(owner, spender);
        if (currentAllowance != type(uint256).max) {
            if (currentAllowance < value) {
                revert ERC20InsufficientAllowance(spender, currentAllowance, value);
            }
            unchecked {
                _approve(owner, spender, currentAllowance - value, false);
            }
        }
    }
}


abstract contract Ownable is Context {
    address private _owner;

    /**
     * @dev The caller account is not authorized to perform an operation.
     */
    error OwnableUnauthorizedAccount(address account);

    /**
     * @dev The owner is not a valid owner account. (eg. `address(0)`)
     */
    error OwnableInvalidOwner(address owner);

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    /**
     * @dev Initializes the contract setting the address provided by the deployer as the initial owner.
     */
    constructor(address initialOwner) {
        if (initialOwner == address(0)) {
            revert OwnableInvalidOwner(address(0));
        }
        _transferOwnership(initialOwner);
    }

    /**
     * @dev Throws if called by any account other than the owner.
     */
    modifier onlyOwner() {
        _checkOwner();
        _;
    }

    /**
     * @dev Returns the address of the current owner.
     */
    function owner() public view virtual returns (address) {
        return _owner;
    }

    /**
     * @dev Throws if the sender is not the owner.
     */
    function _checkOwner() internal view virtual {
        if (owner() != _msgSender()) {
            revert OwnableUnauthorizedAccount(_msgSender());
        }
    }

    /**
     * @dev Leaves the contract without owner. It will not be possible to call
     * `onlyOwner` functions. Can only be called by the current owner.
     *
     * NOTE: Renouncing ownership will leave the contract without an owner,
     * thereby disabling any functionality that is only available to the owner.
     */
    function renounceOwnership() public virtual onlyOwner {
        _transferOwnership(address(0));
    }

    /**
     * @dev Transfers ownership of the contract to a new account (`newOwner`).
     * Can only be called by the current owner.
     */
    function transferOwnership(address newOwner) public virtual onlyOwner {
        if (newOwner == address(0)) {
            revert OwnableInvalidOwner(address(0));
        }
        _transferOwnership(newOwner);
    }

    /**
     * @dev Transfers ownership of the contract to a new account (`newOwner`).
     * Internal function without access restriction.
     */
    function _transferOwnership(address newOwner) internal virtual {
        address oldOwner = _owner;
        _owner = newOwner;
        emit OwnershipTransferred(oldOwner, newOwner);
    }
}


// OpenZeppelin Contracts (last updated v5.1.0) (access/Ownable2Step.sol)



/**
 * @dev Contract module which provides access control mechanism, where
 * there is an account (an owner) that can be granted exclusive access to
 * specific functions.
 *
 * This extension of the {Ownable} contract includes a two-step mechanism to transfer
 * ownership, where the new owner must call {acceptOwnership} in order to replace the
 * old one. This can help prevent common mistakes, such as transfers of ownership to
 * incorrect accounts, or to contracts that are unable to interact with the
 * permission system.
 *
 * The initial owner is specified at deployment time in the constructor for `Ownable`. This
 * can later be changed with {transferOwnership} and {acceptOwnership}.
 *
 * This module is used through inheritance. It will make available all functions
 * from parent (Ownable).
 */
abstract contract Ownable2Step is Ownable {
    address private _pendingOwner;

    event OwnershipTransferStarted(address indexed previousOwner, address indexed newOwner);

    /**
     * @dev Returns the address of the pending owner.
     */
    function pendingOwner() public view virtual returns (address) {
        return _pendingOwner;
    }

    /**
     * @dev Starts the ownership transfer of the contract to a new account. Replaces the pending transfer if there is one.
     * Can only be called by the current owner.
     *
     * Setting `newOwner` to the zero address is allowed; this can be used to cancel an initiated ownership transfer.
     */
    function transferOwnership(address newOwner) public virtual override onlyOwner {
        _pendingOwner = newOwner;
        emit OwnershipTransferStarted(owner(), newOwner);
    }

    /**
     * @dev Transfers ownership of the contract to a new account (`newOwner`) and deletes any pending owner.
     * Internal function without access restriction.
     */
    function _transferOwnership(address newOwner) internal virtual override {
        delete _pendingOwner;
        super._transferOwnership(newOwner);
    }

    /**
     * @dev The new owner accepts the ownership transfer.
     */
    function acceptOwnership() public virtual {
        address sender = _msgSender();
        if (pendingOwner() != sender) {
            revert OwnableUnauthorizedAccount(sender);
        }
        _transferOwnership(sender);
    }
}


contract OsniasClearing_Oeuro is ERC20, Ownable2Step {

    // ─────────────────────────────────────────────────────────────
    // IDENTITÉ DU PROTOCOLE
    // ─────────────────────────────────────────────────────────────

    string public constant PROTOCOL         = "Osnias Clearing";
    string public constant PROTOCOL_VERSION = "1.0";
    string public constant TOKEN_FULL_NAME  = "Osnias Euro Clearing";

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

    /// @notice Gestionnaire initial OEURO sur Sei Testnet.
    address public constant INITIAL_MANAGER =
        0xEAcc9B9e1f18AaE5879e9259684578903d09C4CC;

    /// @notice Wallet de collecte des commissions de 0,15 % sur Sei Testnet.
    address public constant INITIAL_FEE_RECIPIENT =
        0xAD7761445C9CdCC80DFB549E02279353156b5059;

    /**
     * @notice Déploiement de la version de test Sei Testnet.
     * @dev Les adresses initiales sont figées dans ce fichier.
     *      Le gestionnaire pourra ensuite transférer l'ownership via Ownable2Step,
     *      et pourra modifier feeRecipient via setFeeRecipient().
     */
    constructor()
        ERC20("Osnias Euro Clearing", "OEuro")
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
     * @param account Wallet Sei concerné.
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