// Pure CI planning helpers; tools/tasks.mjs remains the only task entry point.
const property = /^(TestProp\w*|TestModelAgreement\w*)$/;
const fuzz = /^Fuzz\w+$/;
const packagePath = /^\.\/internal\/[A-Za-z0-9_/-]+$/;

export function inventory(module, packageLines, events) {
  const packages = new Map();
  for (const line of packageLines.trim().split(/\r?\n/)) {
    const [path, files] = line.split('|');
    if (!path.startsWith(`${module}/internal/`) || files === undefined) throw new Error(`Invalid package record: ${line}`);
    const relative = `.${path.slice(module.length)}`;
    if (!packagePath.test(relative) || packages.has(path)) throw new Error(`Invalid/duplicate package: ${path}`);
    packages.set(path, { package: relative, files: files.split(',').filter(Boolean), tests: [] });
  }
  for (const line of events.trim().split(/\r?\n/).filter(Boolean)) {
    const event = JSON.parse(line);
    if (event.Action === 'fail') throw new Error(`Test discovery failed: ${event.Package}`);
    const name = event.Output?.trim();
    if (event.Action !== 'output' || (!property.test(name) && !fuzz.test(name))) continue;
    const pkg = packages.get(event.Package);
    if (!pkg || pkg.tests.includes(name)) throw new Error(`Invalid/duplicate discovered test: ${event.Package} ${name}`);
    pkg.tests.push(name);
  }
  return [...packages.values()].sort((a, b) => a.package.localeCompare(b.package));
}

export function planChecks(packages, mode = 'pr') {
  if (!['pr', 'nightly'].includes(mode)) throw new Error(`Unknown core CI mode: ${mode}`);
  const checks = [], targets = [];
  for (const pkg of [...packages].sort((a, b) => a.package.localeCompare(b.package))) {
    const tests = pkg.tests.filter(name => property.test(name)).sort();
    const core = pkg.package.startsWith('./internal/core/');
    // Milestone skeletons contain only package documentation. Any other Go file
    // opts a core package into coverage, including a package with no tests yet.
    const coverage = core && pkg.files.some(file => file !== 'doc.go');
    const textop = pkg.package === './internal/core/textop';
    const count = mode === 'pr' ? (textop ? 10000 : 1000) : (textop ? 1000000 : 100000);
    if (mode === 'pr' && (coverage || tests.length)) {
      checks.push({ package: pkg.package, test: '', tests, count, coverage });
    } else if (mode === 'nightly') {
      for (const test of tests) checks.push({ package: pkg.package, test, tests: [test], count, coverage: false });
    }
    for (const test of pkg.tests.filter(name => fuzz.test(name)).sort()) targets.push({ package: pkg.package, test });
  }
  // GitHub's matrix ceiling must fail discovery, not silently omit new tests.
  for (const rows of [checks, targets]) {
    if (rows.length > 256) throw new Error('Core CI matrix exceeds 256 jobs; partition the workflow');
    rows.forEach((row, index) => { row.id = String(index); });
  }
  return { checks, fuzz: targets };
}

export function selectChecks(rows, pkg, test) {
  if (!pkg && !test) return rows;
  const found = rows.filter(row => row.package === pkg && row.test === (test || ''));
  if (found.length !== 1) throw new Error(`No unique discovered check: ${pkg} ${test || ''}`);
  return found;
}

export function propertyArgs(check, mode) {
  if (!check.tests.length) return [];
  return ['test', '-race', '-count=1', mode === 'nightly' ? '-timeout=330m' : '-timeout=20m',
    check.package, `-run=^(${check.tests.join('|')})$`, `-rapid.checks=${check.count}`];
}

export function fuzzArgs(target, duration = '60s') {
  const match = /^([1-9]\d*)(s|m|h)$/.exec(duration);
  const seconds = match ? Number(match[1]) * ({ s: 1, m: 60, h: 3600 })[match[2]] : 0;
  if (!seconds || seconds > 3600) throw new Error('FUZZ_TIME must be a positive whole s/m/h duration, at most 1h');
  return ['test', '-race', '-run=^$', `-fuzz=^${target.test}$`, `-fuzztime=${duration}`,
    '-parallel=2', '-timeout=75m', target.package];
}

export function requireCoverage(profile) {
  const lines = profile.trim().split(/\r?\n/);
  if (lines.shift() !== 'mode: atomic') throw new Error('Expected race-compatible atomic coverage');
  let total = 0, covered = 0;
  for (const line of lines) {
    const match = /^.+:\d+\.\d+,\d+\.\d+ (\d+) (\d+)$/.exec(line);
    if (!match) throw new Error(`Malformed coverage row: ${line}`);
    const statements = Number(match[1]);
    total += statements;
    if (Number(match[2]) > 0) covered += statements;
  }
  // Count statements, not blocks or rounded go-tool output. Empty profiles must
  // not let an implemented package without tests appear green.
  if (!total || covered * 100 <= total * 90) throw new Error(`Core coverage must exceed 90%: ${covered}/${total} statements`);
  return 100 * covered / total;
}
