// Portable task runner: every Makefile recipe also runs directly in PowerShell.
import { execFileSync, spawnSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { tmpdir, cpus } from 'node:os';
import { fileURLToPath } from 'node:url';
import { compose, integrationEnv, up } from './dev.mjs';
import { inventory, planChecks, selectChecks, propertyArgs, fuzzArgs, requireCoverage } from './core-ci.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
process.chdir(root);
const versions = JSON.parse(readFileSync('tools/versions.json', 'utf8'));
const extension = process.platform === 'win32' ? '.exe' : '';
const binary = name => resolve('.tools', name + extension);
const npmCLI = join(dirname(process.execPath), 'node_modules/npm/bin/npm-cli.js');

function run(command, args, options = {}) {
  console.log(`> ${command} ${args.join(' ')}`);
  execFileSync(command, args, { stdio: 'inherit', ...options });
}

function capture(command, args, options = {}) {
  return execFileSync(command, args, { encoding: 'utf8', ...options });
}

function npm(...args) {
  const options = { cwd: resolve('plugin') };
  if (process.platform === 'win32') run(process.execPath, [npmCLI, ...args], options);
  else run('npm', args, options);
}

function files(directory, suffix) {
  return readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const path = join(directory, entry.name);
    return entry.isDirectory() ? files(path, suffix) : path.endsWith(suffix) ? [path] : [];
  });
}

function corePlan() {
  const module = capture('go', ['list', '-m']).trim();
  const packages = capture('go', ['list', '-f', '{{.ImportPath}}|{{join .GoFiles ","}}', './internal/...']);
  const events = capture('go', ['test', '-json', '-list', '^(TestProp|TestModelAgreement|Fuzz)', './internal/...'], { maxBuffer: 16 * 1024 * 1024, env: unitEnv() });
  return planChecks(inventory(module, packages, events), process.env.CORE_MODE || 'pr');
}

function unitEnv() {
  // Never let a local compatibility export or ambient service credentials turn
  // a CI discovery/unit invocation into external I/O.
  return { ...process.env, GARAGE_ENDPOINT: '', STELE_PULL_REQUIRE_S3: '', ANNOT_SAMPLE: '', ANNOT_OUTPUT_DIR: '' };
}

function depguardProof() {
  // An isolated throwaway module proves rejection without dirtying the worktree.
  const scratch = mkdtempSync(join(tmpdir(), 'stele-depguard-'));
  try {
    const goVersion = readFileSync('go.mod', 'utf8').match(/^go (.+)$/m)[1];
    writeFileSync(join(scratch, 'go.mod'), `module github.com/BarneyLaw/stele-sync\n\ngo ${goVersion}\n`);
    writeFileSync(join(scratch, '.golangci.yml'), readFileSync('.golangci.yml'));
    for (const pkg of ['core/textop', 'engine', 'proto', 'storage/pg']) {
      mkdirSync(join(scratch, 'internal', pkg), { recursive: true });
      writeFileSync(join(scratch, 'internal', pkg, 'doc.go'), `package ${pkg.split('/').at(-1)}\n`);
    }
    for (const pkg of ['core/textop', 'engine', 'proto']) {
      const probe = join(scratch, 'internal', pkg, 'forbidden.go');
      writeFileSync(probe, `package ${pkg.split('/').at(-1)}\nimport _ "github.com/BarneyLaw/stele-sync/internal/storage/pg"\n`);
      const result = spawnSync(binary('golangci-lint'), ['run', '--enable-only', 'depguard', './...'], {
        cwd: scratch, encoding: 'utf8', env: { ...process.env, GOWORK: 'off' },
      });
      const output = `${result.stdout || ''}\n${result.stderr || ''}`;
      if (result.status !== 1 || !output.includes('depguard') || !output.includes('storage/pg')) {
        throw new Error(`depguard failed to reject ${pkg} -> storage/pg:\n${output}`);
      }
      rmSync(probe);
    }
    console.log('depguard rejected core, engine, and proto imports of storage/pg.');
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

const tasks = {
  tools() {
    mkdirSync('.tools', { recursive: true });
    const env = { ...process.env, GOBIN: resolve('.tools') };
    run('go', ['install', `github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${versions['golangci-lint']}`], { env });
    run('go', ['install', `golang.org/x/vuln/cmd/govulncheck@${versions.govulncheck}`], { env });
    run('go', ['install', `github.com/rhysd/actionlint/cmd/actionlint@${versions.actionlint}`], { env });
  },
  lint() {
    run(binary('actionlint'), ['-shellcheck=', '-pyflakes=']);
    run(process.execPath, ['--test', 'tools/core-ci.test.mjs', 'tools/workflows.test.mjs']);
    run('go', ['mod', 'tidy', '-diff']);
    const unformatted = capture('gofmt', ['-l', 'cmd', 'internal']);
    if (unformatted.trim()) throw new Error(`Run gofmt on:\n${unformatted}`);
    run('go', ['vet', './...']);
    run(binary('golangci-lint'), ['config', 'verify']);
    run(binary('golangci-lint'), ['run', './...']);
    depguardProof();
  },
  test() {
    tasks['test-unit']();
    tasks['core-check']();
  },
  'test-unit'() {
    run('go', ['test', '-race', '-count=1', './...'], { env: unitEnv() });
    run(process.execPath, ['--test', 'tools/core-ci.test.mjs', 'tools/workflows.test.mjs', 'tools/oracle/generate.test.mjs', 'tools/oracle/report.test.mjs']);
  },
  'core-plan'() {
    const plan = corePlan();
    mkdirSync('.cache/core-ci', { recursive: true });
    writeFileSync('.cache/core-ci/plan.json', JSON.stringify(plan, null, 2) + '\n');
    if (process.env.GITHUB_OUTPUT) {
      for (const [name, include] of Object.entries(plan)) {
        appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${JSON.stringify({ include })}\nhas-${name}=${include.length > 0}\n`);
      }
    }
    console.log(JSON.stringify(plan, null, 2));
  },
  'core-check'() {
    const mode = process.env.CORE_MODE || 'pr';
    const checks = selectChecks(corePlan().checks, process.env.CORE_PACKAGE, process.env.CORE_TEST);
    for (const check of checks) {
      if (check.coverage) {
        const directory = join('.cache/core-ci', check.package.slice(2));
        mkdirSync(directory, { recursive: true });
        const profile = join(directory, 'coverage.out');
        run('go', ['test', '-race', '-count=1', '-timeout=20m', check.package, `-coverprofile=${profile}`], { env: unitEnv() });
        const percent = requireCoverage(readFileSync(profile, 'utf8'));
        console.log(`${check.package}: ${percent.toFixed(2)}% coverage (>90% required)`);
      }
      const args = propertyArgs(check, mode);
      if (args.length) run('go', args, { env: unitEnv() });
    }
  },
  'core-fuzz'() {
    const targets = selectChecks(corePlan().fuzz, process.env.CORE_PACKAGE, process.env.CORE_TEST);
    for (const target of targets) run('go', fuzzArgs(target, process.env.FUZZ_TIME || '60s'), { env: unitEnv() });
  },
  'oracle-scale'() {
    run('go', ['test', '-tags=oracle_scale', '-count=1', '-timeout=115m', '-v', './internal/core/textop', '-run=^TestOracleScale$']);
  },
  'oracle-replay'() {
    run('go', ['test', '-tags=oracle_scale', '-count=1', '-timeout=5m', '-v', './internal/core/textop', '-run=^TestOracleReplay$']);
  },
  'textop-acceptance'() {
    run('go', ['test', '-race', '-count=1', '-timeout=12h', './internal/core/textop', '-run=^TestProp', '-rapid.checks=1000000']);
    run('go', ['test', '-race', '-count=1', './internal/core/textop', '-coverprofile=coverage.textop.out']);
    const coverage = capture('go', ['tool', 'cover', '-func=coverage.textop.out']);
    console.log(coverage);
    const total = coverage.match(/total:.*?([\d.]+)%/);
    if (!total || Number(total[1]) <= 90) throw new Error('textop coverage must exceed 90%');
    for (const target of ['FuzzParse', 'FuzzInsertEscapes', 'FuzzApply']) {
      run('go', ['test', '-race', '-run=^$', `-fuzz=^${target}$`, '-fuzztime=10m', '-timeout=15m', './internal/core/textop']);
    }
  },
  'textop-perf'() {
    let performanceFailure;
    try {
      run('go', ['test', '-count=1', '-timeout=30m', '-v', './internal/core/textop', '-run=^TestTextopPerformance$'], {
        env: { ...process.env, TEXTOP_PERF: '1', TEXTOP_CPU: cpus()[0]?.model || 'unknown' },
      });
    } catch (error) { performanceFailure = error; }
    run('go', ['test', '-run=^$', '-bench=Benchmark(Apply|TransformGap)', '-benchmem', '-count=5', './internal/core/textop']);
    if (performanceFailure) throw performanceFailure;
  },
  'annot-acceptance'() {
    run('go', ['test', '-count=1', '-timeout=90m', './internal/core/annot', '-run=^(TestProp|TestModelAgreement)', '-rapid.checks=100000']);
    const scratch = mkdtempSync(join(tmpdir(), 'stele-annot-acceptance-'));
    try {
      const profile = join(scratch, 'coverage.out');
      run('go', ['test', '-race', '-count=1', './internal/core/annot', `-coverprofile=${profile}`]);
      const coverage = capture('go', ['tool', 'cover', `-func=${profile}`]);
      console.log(coverage);
      const total = coverage.match(/total:.*?([\d.]+)%/);
      if (!total || Number(total[1]) <= 90) throw new Error('annot coverage must exceed 90%');
    } finally { rmSync(scratch, { recursive: true, force: true }); }
    for (const target of ['FuzzParse', 'FuzzParseOps', 'FuzzDecodeState', 'FuzzApply']) {
      run('go', ['test', '-race', '-run=^$', `-fuzz=^${target}$`, '-fuzztime=10m', '-timeout=15m', './internal/core/annot']);
    }
  },
  'test-integration'() {
    up();
    const result = compose('exec', '-T', 'postgres', 'psql', '-U', 'obsync', '-d', 'obsync', '-Atc', 'SELECT 1');
    if (result.trim() !== '1') throw new Error('Postgres query did not return 1');
    run('go', ['test', '-race', '-count=1', '-v', '-run', 'S3', './internal/storage/objects', './internal/run'], { env: integrationEnv() });
    // Real repository/locking tests arrive in M5.
    const pgTests = files('internal/storage/pg', '_test.go');
    if (pgTests.length) run('go', ['test', '-race', '-count=1', '-tags=integration', './internal/storage/pg'], { env: integrationEnv() });
    else console.log('M0: Postgres reachability verified; repository tests deferred to M5.');
  },
  'fuzz-short'() {
    tasks['core-fuzz']();
  },
  sim() {
    if (existsSync('tools/sim/main.go')) run('go', ['run', './tools/sim', '-runs', process.env.SIM_RUNS || '500']);
    else console.log('M0: simulation deferred to M7 (tools/sim/main.go).');
  },
  fixtures() {
    run('go', ['test', './internal/manifest', '-run', '^TestContractFixtures$', '-count=1', '-update']);
    run('go', ['test', './internal/core/annot', '-run', '^TestAnnotContractFixtures$', '-count=1', '-update-annot']);
    if (existsSync('tools/oracle/generate.mjs')) run(process.execPath, ['tools/oracle/generate.mjs']);
  },
  'fixtures-check'() {
    run('go', ['test', './internal/core/annot', '-run', '^TestAnnotContractFixtures$', '-count=1']);
    // Preserve phase 1's regeneration gate. Oracle drift checks never overwrite
    // committed goldens, and detect missing/untracked files explicitly.
    const paths = ['schema/textop/apply.json', 'schema/textop/compose.json', 'schema/textop/transform.json', 'schema/textop/invalid.json',
      ...['parse', 'diff', 'apply', 'state', 'invalid'].map(name => `schema/annot/${name}.json`)];
    for (const path of paths) if (!existsSync(path)) throw new Error(`Missing required fixture ${path}`);
    const tracked = capture('git', ['ls-files', '--', ...paths]).trim().split(/\r?\n/);
    for (const path of paths) if (!tracked.includes(path)) throw new Error(`Fixture must be committed: ${path}`);
    const phase1 = () => new Map(files('schema', '.json').filter(path => !path.includes(`textop`)).map(path => [path, readFileSync(path)]));
    const before = phase1();
    run('go', ['test', './internal/manifest', '-run', '^TestContractFixtures$', '-count=1', '-update']);
    const after = phase1();
    if (before.size !== after.size || [...before].some(([path, bytes]) => !after.get(path)?.equals(bytes))) throw new Error('Phase 1 fixture drift');
    const scratch = mkdtempSync(join(tmpdir(), 'stele-oracle-drift-'));
    try {
      for (const pass of ['first', 'second']) run(process.execPath, ['tools/oracle/generate.mjs', '--out', join(scratch, pass)]);
      for (const kind of ['apply', 'compose', 'transform']) {
        const first = readFileSync(join(scratch, 'first', `${kind}.json`));
        if (!first.equals(readFileSync(join(scratch, 'second', `${kind}.json`))) || !first.equals(readFileSync(`schema/textop/${kind}.json`))) {
          throw new Error(`Oracle fixture drift: ${kind}; regenerate and review`);
        }
      }
    } finally { rmSync(scratch, { recursive: true, force: true }); }
  },
  contract() {
    run('go', ['test', '-race', '-count=1', './internal/manifest', './internal/core/policy']);
    run('go', ['test', '-race', '-count=1', './internal/core/annot', '-run', '^TestAnnotContractFixtures$']);
    npm('exec', '--', 'vitest', 'run', 'src/contract.test.ts', 'src/preview.test.ts', 'src/policy.test.ts');
  },
  plugin() {
    npm('run', 'lint', '--', '--max-warnings=0');
    npm('exec', '--', 'tsc', '--noEmit', '-p', 'tsconfig.core.json');
    npm('test');
    npm('run', 'build');
    const read = path => JSON.parse(readFileSync(`plugin/${path}`, 'utf8'));
    const manifest = read('manifest.json');
    if (read('package.json').version !== manifest.version || read('versions.json')[manifest.version] !== manifest.minAppVersion) {
      throw new Error('Plugin release metadata versions disagree');
    }
  },
  audit() {
    run(binary('govulncheck'), ['./...']);
    npm('audit');
  },
  e2e() {
    if (existsSync('tools/e2e/main.go')) run('go', ['run', './tools/e2e']);
    else console.log('M0: real-server end-to-end gate deferred to M14.');
  },
  worker() {
    run('docker', ['build', '-t', 'stele-pull-worker:m0', '.']);
    run('docker', ['run', '--rm', '--read-only', 'stele-pull-worker:m0', 'help']);
    run('docker', ['run', '--rm', '--read-only', '--tmpfs', '/tmp', '--entrypoint', '/usr/local/bin/stele-pull', 'stele-pull-worker:m0', '-fs-store', '/tmp', 'ls']);
    for (const [GOOS, GOARCH] of [['windows', 'amd64'], ['darwin', 'arm64']]) {
      run('go', ['build', './cmd/stele-pull', './cmd/stele-pull-worker'], { env: { ...process.env, GOOS, GOARCH, CGO_ENABLED: '0' } });
    }
    const bash = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : 'bash';
    run(bash, ['scripts/render-deploy.sh', 'deploy/apps/garage-obsync', 'deploy/apps/obsync-worker']);
  },
  ci() {
    for (const name of ['lint', 'test', 'fixtures-check', 'contract', 'fuzz-short', 'sim', 'plugin', 'audit', 'test-integration', 'worker']) tasks[name]();
  },
};

const task = process.argv[2];
try {
  if (!Object.hasOwn(tasks, task)) throw new Error(`Usage: node tools/tasks.mjs <${Object.keys(tasks).join('|')}>`);
  tasks[task]();
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
