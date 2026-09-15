OSNIAS CLEARING — CORPORATE KYC TESTNET v0.1.0

FILES TO ADD
- /kyc.html
- /assets/kyc.css
- /assets/kyc.js

EXISTING REPO DEPENDENCIES
- /assets/end-user.css (v1.3.0 compatible)
- /assets/wallet-connect.js (v1.2.0 compatible)
- /logo.jpg

FLOW
1. User connects an EIP-1193 wallet on Ethereum Sepolia.
2. The corporate KYC form is unlocked.
3. On submission, the wallet signs a KYC registration message (no transaction).
4. A provisional corporate UID is reserved: KK-NNN-SSSSSSS.
5. Status remains PENDING_NODE_VALIDATION.
6. After Node validation, the same UID becomes definitive/active.
7. Only then should the platform expose the clearing-room forms (USD 1W / 4W / 13W etc.).

IMPORTANT — TESTNET VS PRODUCTION
By default, submitEndpoint is empty. In that mode, kyc.js generates a local test UID solely for UI/testnet work.
For production, the Node backend must allocate SSSSSSS authoritatively, store KYC data off-chain, send the confirmation e-mail, and return e.g.:
{
  "caseReference": "KYC-...",
  "provisionalUid": "01-001-1234567",
  "status": "PENDING_NODE_VALIDATION"
}

Set in kyc.html:
window.OSNIAS_KYC_CONFIG.submitEndpoint = "/api/kyc/applications";

SECURITY BOUNDARY
Do not store KYC evidence or personal data on a public blockchain. The wallet signature proves control of the wallet only; it is not KYC approval.
