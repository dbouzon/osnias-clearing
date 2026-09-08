// SPDX-License-Identifier: BUSL-1.1
pragma solidity ^0.8.26;

import {Ownable2Step} from "@openzeppelin/contracts/access/Ownable2Step.sol";
import {ReentrancyGuard} from "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @notice Minimal client escrow interface used by the Burn Oracle.
/// @dev The concrete escrow can evolve independently as long as this boundary is preserved.
interface IOsniasClientEscrow {
    /// @notice Total collateral currently attributed to the client escrow account.
    function escrowBalance(bytes32 osniasId, bytes32 escrowId) external view returns (uint256);

    /// @notice Amount not already reserved by another pending operation.
    function availableBalance(bytes32 osniasId, bytes32 escrowId) external view returns (uint256);

    /// @notice Reserve collateral for this burn request without releasing it.
    function reserveForBurn(
        bytes32 burnRequestId,
        bytes32 osniasId,
        bytes32 escrowId,
        uint256 amount
    ) external;

    /// @notice Release the previously reserved collateral to the client.
    /// @return releaseRef EVM-side immutable transaction/reference identifier.
    function releaseForBurn(bytes32 burnRequestId) external returns (bytes32 releaseRef);

    /// @notice Cancel a still-unreleased reservation.
    function cancelBurnReservation(bytes32 burnRequestId) external;
}

/// @title OsniasEvmBurnOracleController
/// @notice EVM-side symmetric burn oracle controller.
/// @dev V0.1: receives authenticated Sei messages from a configured relay,
///      verifies strict escrow sufficiency, reserves collateral, releases or cancels.
contract OsniasEvmBurnOracleController is Ownable2Step, ReentrancyGuard {
    enum Status {
        NONE,
        RESERVED,
        RELEASED,
        CANCELLED
    }

    struct BurnReservation {
        bytes32 osniasId;
        bytes32 escrowId;
        uint256 amount;
        uint256 seiChainId;
        uint64 seiDeadline;
        Status status;
    }

    IOsniasClientEscrow public immutable clientEscrow;

    /// @notice Authenticated relay for messages received from the Sei Burn Oracle.
    address public seiOracleRelay;

    /// @notice Operator allowed to execute a collateral release after reservation.
    /// @dev Can later be replaced by stronger threshold/attestation logic.
    address public releaseOperator;

    mapping(bytes32 => BurnReservation) public reservations;
    mapping(bytes32 => bool) public consumedMessages;

    event SeiOracleRelayUpdated(address indexed oldRelay, address indexed newRelay);
    event ReleaseOperatorUpdated(address indexed oldOperator, address indexed newOperator);

    event BurnReserved(
        bytes32 indexed burnRequestId,
        bytes32 indexed osniasId,
        bytes32 indexed escrowId,
        uint256 amount,
        uint64 seiDeadline
    );

    /// @notice Sent back through the encrypted channel so Sei knows the request exists on EVM.
    event SeiReservationAccepted(bytes32 indexed burnRequestId, uint256 amount);

    event CollateralReleased(
        bytes32 indexed burnRequestId,
        uint256 amount,
        bytes32 indexed releaseRef
    );

    /// @notice Message intended for Sei: exact collateral release has occurred.
    event SeiReleaseConfirmationRequested(
        bytes32 indexed burnRequestId,
        uint256 amount,
        bytes32 indexed releaseRef
    );

    event ReservationCancelled(bytes32 indexed burnRequestId);

    /// @notice Message intended for Sei: collateral remains unreleased and request is closed.
    event SeiCancellationConfirmationRequested(bytes32 indexed burnRequestId);

    error ZeroAddress();
    error InvalidRequest();
    error InvalidStatus();
    error UnauthorizedOracle();
    error UnauthorizedOperator();
    error MessageAlreadyConsumed();
    error InsufficientEscrow();
    error DeadlinePassed();

    modifier onlySeiOracleRelay() {
        if (msg.sender != seiOracleRelay) revert UnauthorizedOracle();
        _;
    }

    modifier onlyReleaseOperator() {
        if (msg.sender != releaseOperator) revert UnauthorizedOperator();
        _;
    }

    constructor(
        address initialOwner,
        address escrowContract,
        address initialSeiOracleRelay,
        address initialReleaseOperator
    ) Ownable(initialOwner) {
        if (
            initialOwner == address(0) ||
            escrowContract == address(0) ||
            initialSeiOracleRelay == address(0) ||
            initialReleaseOperator == address(0)
        ) revert ZeroAddress();

        clientEscrow = IOsniasClientEscrow(escrowContract);
        seiOracleRelay = initialSeiOracleRelay;
        releaseOperator = initialReleaseOperator;
    }

    function setSeiOracleRelay(address newRelay) external onlyOwner {
        if (newRelay == address(0)) revert ZeroAddress();
        address old = seiOracleRelay;
        seiOracleRelay = newRelay;
        emit SeiOracleRelayUpdated(old, newRelay);
    }

    function setReleaseOperator(address newOperator) external onlyOwner {
        if (newOperator == address(0)) revert ZeroAddress();
        address old = releaseOperator;
        releaseOperator = newOperator;
        emit ReleaseOperatorUpdated(old, newOperator);
    }

    /// @notice Receives BURN_OPEN from Sei.
    /// @dev Critical business rule: amount MUST be strictly lower than the
    ///      escrow's currently available collateral.
    function receiveBurnOpen(
        bytes32 burnRequestId,
        bytes32 osniasId,
        bytes32 escrowId,
        uint256 amount,
        uint256 seiChainId,
        uint64 seiDeadline,
        bytes32 messageId
    ) external onlySeiOracleRelay nonReentrant {
        _consumeMessage(messageId);

        if (
            burnRequestId == bytes32(0) ||
            osniasId == bytes32(0) ||
            escrowId == bytes32(0) ||
            amount == 0
        ) revert InvalidRequest();

        if (reservations[burnRequestId].status != Status.NONE) revert InvalidStatus();
        if (block.timestamp >= seiDeadline) revert DeadlinePassed();

        uint256 available = clientEscrow.availableBalance(osniasId, escrowId);

        // User requirement: burn request must be STRICTLY LESS than escrow amount.
        if (amount >= available) revert InsufficientEscrow();

        reservations[burnRequestId] = BurnReservation({
            osniasId: osniasId,
            escrowId: escrowId,
            amount: amount,
            seiChainId: seiChainId,
            seiDeadline: seiDeadline,
            status: Status.RESERVED
        });

        clientEscrow.reserveForBurn(burnRequestId, osniasId, escrowId, amount);

        emit BurnReserved(burnRequestId, osniasId, escrowId, amount, seiDeadline);
        emit SeiReservationAccepted(burnRequestId, amount);
    }

    /// @notice Executes the exact reserved collateral release.
    /// @dev V0.1 operator hook. Future version can replace this with richer
    ///      automated lender/escrow attestation logic.
    function releaseCollateral(bytes32 burnRequestId)
        external
        onlyReleaseOperator
        nonReentrant
        returns (bytes32 releaseRef)
    {
        BurnReservation storage r = reservations[burnRequestId];
        if (r.status != Status.RESERVED) revert InvalidStatus();

        // Do not begin a release after Sei's advertised 15-minute deadline.
        if (block.timestamp >= r.seiDeadline) revert DeadlinePassed();

        // Effects before interaction.
        r.status = Status.RELEASED;
        releaseRef = clientEscrow.releaseForBurn(burnRequestId);

        emit CollateralReleased(burnRequestId, r.amount, releaseRef);
        emit SeiReleaseConfirmationRequested(burnRequestId, r.amount, releaseRef);
    }

    /// @notice Receives BURN_CLOSE from Sei after the 15-minute window.
    /// @dev If already RELEASED, this MUST NOT cancel. The relay must instead
    ///      propagate RELEASED back to Sei, which causes the Sei token burn.
    function receiveBurnClose(
        bytes32 burnRequestId,
        bytes32 messageId
    ) external onlySeiOracleRelay nonReentrant {
        _consumeMessage(messageId);

        BurnReservation storage r = reservations[burnRequestId];

        if (r.status == Status.RELEASED) {
            // Release won the race. Never cancel / never claim "not released".
            revert InvalidStatus();
        }
        if (r.status != Status.RESERVED) revert InvalidStatus();

        r.status = Status.CANCELLED;
        clientEscrow.cancelBurnReservation(burnRequestId);

        emit ReservationCancelled(burnRequestId);
        emit SeiCancellationConfirmationRequested(burnRequestId);
    }

    function _consumeMessage(bytes32 messageId) internal {
        if (messageId == bytes32(0)) revert InvalidRequest();
        if (consumedMessages[messageId]) revert MessageAlreadyConsumed();
        consumedMessages[messageId] = true;
    }
}
