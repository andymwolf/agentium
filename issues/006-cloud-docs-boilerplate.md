# Extract common content from cloud setup documentation

## Problem

Cloud setup docs share ~40% common content across providers, wasting ~1,500 tokens:
- `docs/cloud-setup/aws.md` (~240 lines)
- `docs/cloud-setup/azure.md` (~240 lines)
- `docs/cloud-setup/gcp.md` (~230 lines)

### Shared Content
- Prerequisites sections
- Authentication concepts
- General workflow steps
- Common troubleshooting

## Proposed Solution

1. Create `docs/cloud-setup/COMMON.md` with shared content:
   - General prerequisites
   - Authentication overview
   - Common workflow steps
   - Shared troubleshooting
2. Update provider-specific docs to reference common content
3. Keep only provider-specific details in aws.md, azure.md, gcp.md
4. Target: reduce each file from ~240 lines to ~150 lines

## Impact

- **Token savings:** ~1,500 tokens
- **Effort:** Low-Medium
- **Risk:** Low - documentation only

## Labels
bloat-reduction, documentation
