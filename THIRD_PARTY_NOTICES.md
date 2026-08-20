# TapX Third-Party Notices

TapX uses third-party software through declared package and module dependencies.
These notices apply only to the identified material and do not transfer the
TapX copyright statement to third-party work.

## Web runtime packages

The production Web bundle includes React, Ant Design, Day.js, QR Code for
React, and their runtime dependencies. The exact package
versions, source locations, copyright notices, and license texts are generated
from `web/package-lock.json` into:

- `web/THIRD_PARTY_LICENSES.txt`

`npm run licenses:check` fails when that file is missing or stale.

The connector speed test is implemented by TapX's provider-neutral backend and
uses public network-quality HTTP endpoints. No Cloudflare Speedtest SDK or npm
package is embedded in the Web bundle. Service terms, privacy requirements, and
attribution for each configured provider are reviewed separately from software
dependency licenses.

## Xray and Go modules

TapX uses the unmodified official `github.com/xtls/xray-core` module and other
Go modules. Xray-core is MPL-2.0. The current linked dependency graph also
contains `github.com/juju/ratelimit` under LGPL-3.0 with its stated linking
exception; the generated dependency notices contain no GPL-3.0-or-later Go
module. The exact Linux `tapx-core` and `tapx-panel` module versions, binary
usage, source locations, and original license texts are generated into:

- `GO_THIRD_PARTY_LICENSES.txt`

The complete module versions, source locations, copyright notices, and license
texts are included in `GO_THIRD_PARTY_LICENSES.txt` inside each release archive.
The proprietary license for TapX-owned material is provided in the repository
root `LICENSE` file and does not replace or narrow any third-party license.
