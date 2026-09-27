// Deterministic triage: preserve evidence, group symptoms, update ONE rolling
// issue. No model credentials, auto-fixes, or one-issue-per-counterexample fanout.
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';

const MARKER = '<!-- stele-nightly-oracle:v1 -->';
const LAST_FAILURE = '<!-- stele-oracle-last-failure -->';
const TITLE = 'Nightly textop oracle: rolling failure report';

export function classify(failure) {
  const diagnostic = failure.diagnostic ?? '';
  if (!failure.case || !['apply', 'compose', 'transform'].includes(failure.case.kind)) return ['infrastructure', 'The oracle stream did not complete its contract. Inspect dependency setup, generator stderr, timeout, and record/count diagnostics.'];
  if (/components differ/.test(diagnostic)) return [`${failure.case.kind}/components`, 'The Go algebra returned different ordered components from pinned ot.js. Equal final text would not make this mismatch acceptable.'];
  if (/document differs|paths differ|scalar model/.test(diagnostic)) return [`${failure.case.kind}/document`, 'The resulting text disagreed with ot.js or the independent scalar edit model. Compare exact bytes, UTF-16 boundaries, and both application paths.'];
  if (/length/.test(diagnostic)) return [`${failure.case.kind}/lengths`, 'An input or result violated the declared UTF-16 length contract. The saved case contains the complete operation and base text.'];
  return [`${failure.case.kind}/contract`, 'A schema, canonicality, boundary, coverage, or operation error check failed. The diagnostic is evidence of the failed check, not a root-cause claim.'];
}

function artifactFiles(directory) {
  if (!existsSync(directory)) return [];
  return readdirSync(directory, { withFileTypes: true }).flatMap(e => e.isDirectory() ? artifactFiles(join(directory, e.name)) : [join(directory, e.name)]);
}

// Escaping prevents generated text from injecting Markdown mentions or markup.
const safe = s => String(s).replace(/[<>&@`\[\]]/g, c => `&#${c.charCodeAt(0)};`).replace(/\r?\n/g, ' ');

export function collate(directory, runURL, result) {
  const failures = [], summaries = [], problems = [];
  for (const file of artifactFiles(directory).sort()) {
    try {
      if (statSync(file).size > 128 * 1024 * 1024) throw new Error('artifact exceeds 128 MiB ceiling');
      if (file.endsWith('oracle-failures.ndjson')) {
        for (const line of readFileSync(file, 'utf8').trim().split('\n')) {
          if (!line) continue;
          if (failures.length >= 16) throw new Error('too many failure records');
          const f = JSON.parse(line);
          if (typeof f.diagnostic !== 'string') throw new Error('missing diagnostic');
          failures.push({ ...f, artifact: file });
        }
      } else if (file.endsWith('oracle-summary.json')) summaries.push(JSON.parse(readFileSync(file, 'utf8')));
    } catch (e) { problems.push(`${file}: ${e.message}`); }
  }
  const groups = new Map();
  for (const f of failures) {
    const [key, explanation] = classify(f);
    if (!groups.has(key)) groups.set(key, { explanation, failures: [] });
    groups.get(key).failures.push(f);
  }
  const failed = result !== 'success' || failures.length > 0 || problems.length > 0 || summaries.length === 0 || summaries.some(s => s.passed !== true);
  const lines = [MARKER, `Latest run: [GitHub Actions](${runURL})`, '', `Status: **${failed ? 'needs investigation' : 'passed'}**. This issue is updated in place; prior runs remain in Actions artifacts.`, ''];
  if (summaries.length) {
    lines.push('| Seed | Cases checked | Apply / compose / transform | Exact 1 MiB per kind | ≥500 components per kind |', '| --- | ---: | --- | --- | --- |');
    for (const s of summaries.slice(0, 4)) {
      const c = s.coverage ?? {}, kinds = ['apply', 'compose', 'transform'];
      const counts = key => kinds.map(k => Number(c[key]?.[k] ?? 0)).join(' / ');
      lines.push(`| ${safe(s.header?.seed)} | ${Number(c.count ?? 0)} | ${counts('kinds')} | ${counts('large')} | ${counts('complex')} |`);
    }
    lines.push('');
  }
  for (const [key, group] of groups) {
    lines.push(`### ${safe(key)} — ${group.failures.length} shard(s)`, '', group.explanation, '');
    for (const f of group.failures.slice(0, 4)) {
      lines.push(`- Seed ${safe(f.header?.seed)}, index ${safe(f.case?.i ?? 'unavailable')}, case ${safe(f.case?.id ?? 'unavailable')}, generator ${safe(f.header?.generator_hash ?? 'unavailable')}.`,
        `  Diagnostic: ${safe(f.diagnostic.slice(0, 1200))}`);
    }
    lines.push('');
  }
  if (failed && !failures.length) lines.push('No case artifact was produced. Inspect the failed shard setup/process logs; this is not evidence of an algebra mismatch.', '');
  for (const problem of problems) lines.push(`Artifact error: ${safe(problem)}`, '');
  if (failed) lines.push('Download the `oracle-failures-shard-*` artifact from this run. Replay its complete case without regenerating the prefix:', '',
    '```sh', 'ORACLE_REPLAY=/path/to/oracle-failures.ndjson node tools/tasks.mjs oracle-replay', '```', '',
    'To replay a full shard, dispatch Nightly with its seed and original count. A supplied seed runs one shard. Diagnostics are grouped mechanically; root cause requires investigation.');
  return { failed, body: lines.join('\n').slice(0, 60000) };
}

export async function reportNightly({ github, context, core, directory, result }) {
  const runURL = `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
  const report = collate(directory, runURL, result);
  await core.summary.addRaw(report.body).write();
  // Manual runs on development branches retain summaries/artifacts but do not
  // overwrite the default branch's rolling health report.
  if (context.ref !== `refs/heads/${context.payload.repository.default_branch}`) return;
  const issues = await github.paginate(github.rest.issues.listForRepo, { ...context.repo, state: 'all', creator: 'github-actions[bot]', per_page: 100 });
  const existing = issues.find(i => !i.pull_request && i.body?.includes(MARKER));
  if (existing) {
    // Do not automatically close a failure after a smaller successful manual
    // replay; retain the issue until a maintainer has assessed the evidence.
    let body = report.body;
    if (!report.failed) {
      const previous = existing.body.includes(LAST_FAILURE) ? existing.body.split(LAST_FAILURE)[1] :
        existing.body.includes('**needs investigation**') ? `\n\n## Last failure\n\n${existing.body.replace(MARKER, '')}` : '';
      if (previous) body += `\n\n${LAST_FAILURE}${previous}`;
    }
    await github.rest.issues.update({ ...context.repo, issue_number: existing.number, body: body.slice(0, 60000), ...(report.failed ? { state: 'open' } : {}) });
  } else if (report.failed) {
    await github.rest.issues.create({ ...context.repo, title: TITLE, body: report.body });
  }
}
