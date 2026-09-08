// SPDX-License-Identifier: BUSL-1.1
pragma solidity ^0.8.26;

import {Ownable2Step} from "@openzeppelin/contracts/access/Ownable2Step.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import {IERC20} from "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import {SafeERC20} from "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";

/// @notice Minimal interface expected from an OSNIAS clearing token.
/// @dev The token implementation should authorize this controller as a burn manager.
///      If ORUSD/OEURO exposes a different burn signature, adapt only this interface/call.
interface IClearanceBurnable is IERC20 {
    function burn(uint256 amount) external;
}

/// @title OsniasSeiBurnOracleController
/// @notice Sei-side temporal token escrow and burn settlement controller.
/// @dev V0.1 base: the encrypted transport is off-chain. On-chain, only the
///      configured EVM oracle relay may acknowledge EVM state transitions.
contract OsniasSeiBurnOracleController is Ownable2Step, ReentrancyGuard {
    using SafeERC20 for IERC20;

    uint64 public constant BURN_WINDOW = 15 minutes;

    enum Status {
        NONE,
        LOCKED,
        CLOSING,
        BURNED,
        REFUNDED
    }

    struct BurnRequest {
        address user;
        address token;
        uint256 amount;
        bytes32 osniasId;
        bytes32 escrowId;
        uint256 evmChainId;
        uint64 createdAt;
        uint64 deadline;
        Status status;
    }

    /// @notice Authorized process that relays authenticated messages from the EVM Burn Oracle.
    address public evmOracleRelay;

    /// @notice Monotonic local identifier source.
    uint256 public requestNonce;

    mapping(bytes32 => BurnRequest) public requests;

    /// @notice Cross-domain message IDs already consumed on Sei.
    mapping(bytes32 => bool) public consumedMessages;

    event EvmOracleRelayUpdated(address indexed oldRelay, address indexed newRelay);

    event BurnOpened(
        bytes32 indexed burnRequestId,
        bytes32 indexed osniasId,
        bytes32 indexed escrowId,
        address user,
        address token,
        uint256 amount,
        uint256 evmChainId,
        uint64 deadline
    );

    /// @notice Emitted for the off-chain encrypted oracle channel.
    event EvmOpenRequested(
        bytes32 indexed burnRequestId,
        bytes32 indexed osniasId,
        bytes32 indexed escrowId,
        uint256 amount,
        uint256 seiChainId,
        uint256 evmChainId,
        uint64 deadline
    );

    event BurnClosing(bytes32 indexed burnRequestId);

    /// @notice Emitted at timeout: the EVM side must close/cancel the reservation
    ///         and acknowledge before Sei can refund.
    event EvmCloseRequested(bytes32 indexed burnRequestId);

    event CollateralReleasedConfirmed(
        bytes32 indexed burnRequestId,
        bytes32 indexed messageId,
        bytes32 evmReleaseRef
    );

    event EvmCancellationConfirmed(
        bytes32 indexed burnRequestId,
        bytes32 indexed messageId
    );

    event TokensBurned(bytes32 indexed burnRequestId, uint256 amount);
    event TokensRefunded(bytes32 indexed burnRequestId, address indexed user, uint256 amount);

    error ZeroAddress();
    error ZeroAmount();
    error InvalidRequest();
    error InvalidStatus();
    error UnauthorizedOracle();
    error WindowExpired();
    error WindowStillOpen();
    error MessageAlreadyConsumed();
    error WrongAmount();
    error WrongChain();

    modifier onlyEvmOracleRelay() {
        if (msg.sender != evmOracleRelay) revert UnauthorizedOracle();
        _;
    }

    constructor(address initialOwner, address initialEvmOracleRelay)
        Ownable(initialOwner)
    {
        if (initialOwner == address(0) || initialEvmOracleRelay == address(0)) {
            revert ZeroAddress();
        }
        evmOracleRelay = initialEvmOracleRelay;
    }

    function setEvmOracleRelay(address newRelay) external onlyOwner {
        if (newRelay == address(0)) revert ZeroAddress();
        address old = evmOracleRelay;
        evmOracleRelay = newRelay;
        emit EvmOracleRelayUpdated(old, newRelay);
    }

    /// @notice Locks clearing tokens on Sei for at most 15 minutes before closure starts.
    /// @dev UI should pre-check escrow availability, but the authoritative EVM
    ///      availability check/reservation occurs on the EVM side.
    function openBurn(
        address token,
        uint256 amount,
        bytes32 osniasId,
        bytes32 escrowId,
        uint256 evmChainId
    ) external nonReentrant returns (bytes32 burnRequestId) {
        if (token == address(0)) revert ZeroAddress();
        if (amount == 0) revert ZeroAmount();
        if (osniasId == bytes32(0) || escrowId == bytes32(0)) revert InvalidRequest();

        uint256 nonce = ++requestNonce;
        burnRequestId = keccak256(
            abi.encode(
                block.chainid,
                address(this),
                msg.sender,
                token,
                amount,
                osniasId,
                escrowId,
                evmChainId,
                nonce
            )
        );

        uint64 nowTs = uint64(block.timestamp);
        uint64 deadline = nowTs + BURN_WINDOW;

        requests[burnRequestId] = BurnRequest({
            user: msg.sender,
            token: token,
            amount: amount,
            osniasId: osniasId,
            escrowId: escrowId,
            evmChainId: evmChainId,
            createdAt: nowTs,
            deadline: deadline,
            status: Status.LOCKED
        });

        // The controller itself is the temporal token escrow.
        IERC20(token).safeTransferFrom(msg.sender, address(this), amount);

        emit BurnOpened(
            burnRequestId,
            osniasId,
            escrowId,
            msg.sender,
            token,
            amount,
            evmChainId,
            deadline
        );
        emit EvmOpenRequested(burnRequestId, osniasId, escrowId, amount, block.chainid, evmChainId, deadline);
    }

    /// @notice After 15 minutes, freezes the Sei request in CLOSING and asks EVM to cancel.
    /// @dev Permissionless keeper-friendly trigger. It never refunds by itself.
    function closeExpired(bytes32 burnRequestId) external {
        BurnRequest storage r = requests[burnRequestId];
        if (r.status != Status.LOCKED) revert InvalidStatus();
        if (block.timestamp < r.deadline) revert WindowStillOpen();

        r.status = Status.CLOSING;
        emit BurnClosing(burnRequestId);
        emit EvmCloseRequested(burnRequestId);
    }

    /// @notice EVM oracle confirms that the exact collateral amount was released.
    /// @dev Accepted from LOCKED or CLOSING. If release won the race, tokens MUST burn.
    function confirmCollateralReleased(
        bytes32 burnRequestId,
        uint256 releasedAmount,
        uint256 sourceChainId,
        bytes32 evmReleaseRef,
        bytes32 messageId
    ) external onlyEvmOracleRelay nonReentrant {
        _consumeMessage(messageId);

        BurnRequest storage r = requests[burnRequestId];
        if (r.status != Status.LOCKED && r.status != Status.CLOSING) revert InvalidStatus();
        if (sourceChainId != r.evmChainId) revert WrongChain();
        if (releasedAmount != r.amount) revert WrongAmount();

        r.status = Status.BURNED;

        // Tokens are already held by this controller. Burn only its own locked balance.
        IClearanceBurnable(r.token).burn(r.amount);

        emit CollateralReleasedConfirmed(burnRequestId, messageId, evmReleaseRef);
        emit TokensBurned(burnRequestId, r.amount);
    }

    /// @notice EVM oracle proves reservation was cancelled / collateral was NOT released.
    /// @dev Refund is possible only from CLOSING, never directly from LOCKED.
    function confirmEvmCancelled(
        bytes32 burnRequestId,
        uint256 sourceChainId,
        bytes32 messageId
    ) external onlyEvmOracleRelay nonReentrant {
        _consumeMessage(messageId);

        BurnRequest storage r = requests[burnRequestId];
        if (r.status != Status.CLOSING) revert InvalidStatus();
        if (sourceChainId != r.evmChainId) revert WrongChain();

        r.status = Status.REFUNDED;
        IERC20(r.token).safeTransfer(r.user, r.amount);

        emit EvmCancellationConfirmed(burnRequestId, messageId);
        emit TokensRefunded(burnRequestId, r.user, r.amount);
    }

    function _consumeMessage(bytes32 messageId) internal {
        if (messageId == bytes32(0)) revert InvalidRequest();
        if (consumedMessages[messageId]) revert MessageAlreadyConsumed();
        consumedMessages[messageId] = true;
    }
}
