import { execFileSync } from 'node:child_process';

export function compose(...args) {
  return execFileSync('docker', ['compose', ...args], { encoding: 'utf8' });
}

export function up() {
  execFileSync('docker', ['compose', 'up', '--build', '--detach', '--wait', '--wait-timeout', '120'], { stdio: 'inherit' });
}

export function integrationEnv() {
  const info = compose('exec', '-T', 'garage', '/garage', 'key', 'info', '--show-secret', 'obsync-dev');
  const key = info.match(/key id:\s*(\S+)/i)?.[1];
  const secret = info.match(/secret key:\s*(\S+)/i)?.[1];
  if (!key || !secret) throw new Error('Cannot read local Garage credentials');
  return {
    ...process.env,
    GARAGE_ENDPOINT: `http://127.0.0.1:${process.env.GARAGE_DEV_S3_PORT || '3900'}`,
    GARAGE_BUCKET: 'obsync', GARAGE_REGION: 'garage',
    GARAGE_ACCESS_KEY: key, GARAGE_SECRET_KEY: secret,
    STELE_PULL_REQUIRE_S3: '1',
    DATABASE_URL: `postgres://obsync:obsync-local-only@127.0.0.1:${process.env.POSTGRES_DEV_PORT || '55432'}/obsync?sslmode=disable`,
  };
}

if (process.argv[1]?.endsWith('dev.mjs')) {
  if (process.argv[2] !== 'up') throw new Error('Usage: node tools/dev.mjs up');
  up();
  console.log('Postgres and Garage are ready. Run node tools/tasks.mjs test-integration.');
}
