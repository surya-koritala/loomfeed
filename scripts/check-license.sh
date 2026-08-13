#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

fail() {
    echo "license check failed: $1" >&2
    exit 1
}

require_text() {
    file=$1
    expected=$2
    grep -Fq "$expected" "$project_dir/$file" ||
        fail "$file is missing: $expected"
}

license_text=$(tr '\n' ' ' <"$project_dir/LICENSE" | tr -s ' ')

printf '%s' "$license_text" | grep -Fq \
    'Copyright (c) 2026 RoamXAI, Surya Koritala, and Loomfeed contributors' ||
    fail 'LICENSE must recognize RoamXAI, Surya Koritala, and Loomfeed contributors'
printf '%s' "$license_text" | grep -Fq \
    'Permission is hereby granted, free of charge, to any person obtaining a copy' ||
    fail 'LICENSE does not contain the canonical MIT opening grant'
printf '%s' "$license_text" | grep -Fq \
    'use, copy, modify, merge, publish, distribute, sublicense, and/or sell' ||
    fail 'LICENSE does not contain the complete canonical MIT permission grant'
printf '%s' "$license_text" | grep -Fq \
    'The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.' ||
    fail 'LICENSE does not contain the canonical MIT notice condition'
printf '%s' "$license_text" | grep -Fq \
    'THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND' ||
    fail 'LICENSE does not contain the canonical MIT warranty disclaimer'
printf '%s' "$license_text" | grep -Fq \
    'OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.' ||
    fail 'LICENSE does not contain the complete canonical MIT liability disclaimer'

require_text AUTHORS.md 'Surya Koritala'
require_text AUTHORS.md 'git shortlog -sne --all'
require_text NOTICE 'Copyright (c) 2026 RoamXAI, Surya Koritala, and Loomfeed contributors'

require_text README.md 'Use and copy the software.'
require_text README.md 'Modify and merge it.'
require_text README.md 'Publish and distribute it.'
require_text README.md 'Sublicense and/or sell copies.'
require_text README.md '[Authors and contributors](AUTHORS.md)'

require_text CONTRIBUTING.md '[MIT License](LICENSE)'
require_text web/package.json '"license": "MIT"'
require_text sdks/typescript/package.json '"license": "MIT"'
require_text sdks/python/setup.py 'license="MIT"'

echo 'MIT license text, attribution, documentation, and package metadata are consistent.'
