# Uniconta — Osnias Clearing

This directory reserves the GitHub code location for the future Go connector. **No executable Uniconta connector has been delivered here.**

## Documentation (source of truth: Google Drive)

- [Uniconta API technical dossier — v0.1 — 2026-11-14](https://docs.google.com/document/d/1Cv2DZIxyG9uWnsm8MPj-izRx33gCwv8aeuMwURE8Nkc/edit)
- [Uniconta API documentation folder](https://drive.google.com/drive/folders/13ADtJAavdIUmsa-XYLMFKEceLdoqq9zz)
- [Official Uniconta Web API](https://api.uniconta.com/)
- [Official OpenAPI reference](https://api.uniconta.com/swagger/v1/swagger.json)

## Intended scope

Read-only Go adapter; authorized company context; customer/supplier, invoice and open-transaction extraction; conversion to the Osnias pivot format. No Uniconta write-back, settlement or payment initiation.

Static documentation lives in Google Drive; future application code and tests live on GitHub. Any implementation must be separately reviewed and tested. Do not treat this reference directory as a completed connector.
