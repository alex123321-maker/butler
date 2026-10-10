// Run on the lab against the actual packaged .output, without printing code.
import fs from 'node:fs'
import path from 'node:path'

const root = path.resolve(process.argv[2] || '/app/.output')
const packages = new Map()
const checks = new Map([
  ['node-forge', /(?:['"]node-forge(?:\/[^'"]*)?['"]|forge\.(?:pki|asn1)|_bnToBytes)/],
  ['braces', /(?:['"]braces(?:\/[^'"]*)?['"]|require\(['"]braces['"]\))/],
  ['postcss-selector-parser', /['"]postcss-selector-parser(?:\/[^'"]*)?['"]/],
])
const references = Object.fromEntries([...checks.keys()].map(name => [name, 0]))
let codeFiles = 0
function visit(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.isSymbolicLink()) throw new Error('output audit refuses symlinks')
    const file = path.join(dir, entry.name)
    if (entry.isDirectory()) visit(file)
    else if (entry.isFile()) {
      if (entry.name === 'package.json') {
        const pkg = JSON.parse(fs.readFileSync(file, 'utf8'))
        if (pkg.name && pkg.version) packages.set(pkg.name, pkg.version)
      } else if (/\.(?:cjs|mjs|js)$/.test(entry.name)) {
        codeFiles++
        const code = fs.readFileSync(file, 'utf8')
        for (const [name, pattern] of checks) if (pattern.test(code)) references[name]++
      }
    }
  }
}
visit(root)
const result = {
  generated_at: new Date().toISOString(),
  packages: Object.fromEntries([...packages].sort(([a], [b]) => a.localeCompare(b))),
  code_files: codeFiles,
  selected_package_references: references,
  selected_packages_present: [...checks.keys()].filter(name => packages.has(name)),
  scope: 'Packaged module names and known code references; not a general vulnerability or reachability proof.',
}
process.stdout.write(JSON.stringify(result, null, 2) + '\n')
