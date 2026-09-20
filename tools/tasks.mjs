// Portable task runner: every Makefile recipe also runs directly in PowerShell.
import { execFileSync, spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { compose, integrationEnv, up } from './dev.mjs';

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
  },
  lint() {
    run('go', ['mod', 'tidy', '-diff']);
    const unformatted = capture('gofmt', ['-l', 'cmd', 'internal']);
    if (unformatted.trim()) throw new Error(`Run gofmt on:\n${unformatted}`);
    run('go', ['vet', './...']);
    run(binary('golangci-lint'), ['config', 'verify']);
    run(binary('golangci-lint'), ['run', './...']);
    depguardProof();
  },
  test() {
    // Keep unit tests independent of a developer's ambient Garage credentials.
    const env = { ...process.env, GARAGE_ENDPOINT: '', STELE_PULL_REQUIRE_S3: '' };
    run('go', ['test', '-race', '-count=1', './...'], { env });
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
    const packages = capture('go', ['list', './internal/core/...', './internal/proto/...']).trim().split(/\r?\n/);
    let count = 0;
    for (const pkg of packages) {
      const targets = capture('go', ['test', '-list', '^Fuzz', pkg]).split(/\r?\n/).filter(line => /^Fuzz\w+$/.test(line));
      for (const target of targets) {
        run('go', ['test', '-race', '-run=^$', `-fuzz=^${target}$`, `-fuzztime=${process.env.FUZZ_TIME || '60s'}`, pkg]);
        count++;
      }
    }
    if (!count) console.log('M0: no fuzz targets yet; decoder fuzzing starts in M1–M4.');
  },
  sim() {
    if (existsSync('tools/sim/main.go')) run('go', ['run', './tools/sim', '-runs', process.env.SIM_RUNS || '500']);
    else console.log('M0: simulation deferred to M7 (tools/sim/main.go).');
  },
  fixtures() {
    run('go', ['test', './internal/manifest', '-run', '^TestContractFixtures$', '-count=1', '-update']);
    if (existsSync('tools/oracle/generate.mjs')) run(process.execPath, ['tools/oracle/generate.mjs']);
  },
  'fixtures-check'() {
    const snapshot = () => new Map(files('schema', '.json').map(path => [path, readFileSync(path)]));
    const before = snapshot();
    tasks.fixtures();
    const after = snapshot();
    if (before.size !== after.size || [...before].some(([path, bytes]) => !after.get(path)?.equals(bytes))) {
      throw new Error('Fixture regeneration changed schema/. Review and commit the generated changes.');
    }
  },
  contract() {
    run('go', ['test', '-race', '-count=1', './internal/manifest', './internal/core/policy']);
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
