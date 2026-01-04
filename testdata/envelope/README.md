# Envelope Contract Fixtures

These JSON files define the contract between server and client for API responses.

**Server tests validate output matches these fixtures.**
**Client tests embed matching JSON to verify parsing.**

If you change the envelope format:
1. Update these fixtures
2. Update the embedded JSON in client's `EnvelopeContractTest.kt`
3. Run both server and client tests
4. If either fails, the contract is broken

## Files

| File | Description |
|------|-------------|
| `success.json` | Standard success response with data |
| `success_null_data.json` | Success response without data (e.g., DELETE) |
| `error_simple.json` | Error with `success: false` and `error` field |
| `error_detailed.json` | Error with `code`, `message`, and `details` |

## Required Fields

Every envelope MUST contain:
- `v`: Version number (currently `1`) - the canary field

Success envelopes contain:
- `success`: boolean
- `data`: optional, the response payload

Simple error envelopes contain:
- `success`: false
- `error`: string message

Detailed error envelopes contain:
- `code`: string error code
- `message`: string error message
- `details`: optional object with additional context
