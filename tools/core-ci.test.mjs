import test from 'node:test';
import assert from 'node:assert/strict';
import { inventory, planChecks, selectChecks, propertyArgs, fuzzArgs, requireCoverage } from './core-ci.mjs';

const pkg = (name, tests = [], files = ['logic.go']) => ({ package: `./internal/${name}`, tests, files });
const sample = [
  pkg('core/textop', ['TestPropConvergence', 'FuzzParse']),
  pkg('core/annot', ['TestModelAgreement', 'TestPropDiffApply', 'FuzzParse']),
  pkg('core/policy'), pkg('core/classify', [], ['doc.go']),
];

test('Go discovery uses package-tagged events, not ambiguous repeated target names', () => {
  const module = 'example.test/repo';
  const events = ['core/annot', 'core/textop'].map(name => JSON.stringify({
    Action: 'output', Package: `${module}/internal/${name}`, Output: 'FuzzParse\n',
  })).join('\n');
  const lines = ['core/textop|op.go,doc.go', 'core/annot|apply.go'].map(line => `${module}/internal/${line}`).join('\n');
  const result = inventory(module, lines, events);
  assert.deepEqual(result.map(p => p.tests), [['FuzzParse'], ['FuzzParse']]);
  assert.equal(result[0].package, './internal/core/annot');
  for (const bad of ['{', JSON.stringify({ Action: 'fail', Package: 'broken' }), `${events}\n${events}`]) {
    assert.throws(() => inventory(module, lines, bad));
  }
  assert.throws(() => inventory(module, 'outside/module|file.go', events));
  assert.throws(() => inventory(module, `${lines}\n${lines}`, events));
});

test('PR checks retain M1 budgets, add coverage without requiring existing tests, and skip doc-only skeletons', () => {
  const plan = planChecks(sample);
  assert.equal(plan.checks.length, 3);
  assert.equal(plan.fuzz.length, 2);
  assert(plan.checks.every(c => c.coverage));
  assert.equal(plan.checks.find(c => c.package.endsWith('/textop')).count, 10000);
  assert.equal(plan.checks.find(c => c.package.endsWith('/annot')).count, 1000);
  assert.deepEqual(propertyArgs(plan.checks.find(c => c.package.endsWith('/policy')), 'pr'), []);
  assert.deepEqual(plan, planChecks([...sample].reverse()));
});

test('future core and non-core properties/decoders join automatically', () => {
  const future = [...sample.filter(p => !p.package.endsWith('/classify')), pkg('core/classify', ['TestPropClass']),
    pkg('proto', ['FuzzDecode']), pkg('engine', ['TestModelAgreementRecovery', 'FuzzSubmit'])];
  const plan = planChecks(future);
  assert(plan.checks.some(c => c.package.endsWith('/classify') && c.coverage));
  assert(plan.checks.some(c => c.package.endsWith('/engine') && !c.coverage));
  assert(plan.fuzz.some(c => c.test === 'FuzzDecode'));
  assert(plan.fuzz.some(c => c.test === 'FuzzSubmit'));
});

test('nightly separates each property/model and each fuzz target within runner time limits', () => {
  const plan = planChecks(sample, 'nightly');
  assert.equal(plan.checks.length, 3);
  assert(plan.checks.every(c => c.tests.length === 1 && !c.coverage));
  const textop = plan.checks.find(c => c.package.endsWith('/textop'));
  assert.equal(textop.count, 1000000);
  assert.equal(plan.checks.find(c => c.test === 'TestModelAgreement').count, 100000);
  assert(propertyArgs(textop, 'nightly').includes('-race'));
  assert(propertyArgs(textop, 'nightly').includes('-timeout=330m'));
  assert(propertyArgs(textop, 'nightly').includes('-run=^(TestPropConvergence)$'));
  assert.equal(plan.fuzz.length, 2);
  assert(fuzzArgs(plan.fuzz[0], '1h').includes('-parallel=2'));
  for (const value of ['0s', '2h', '3601s', '1.5s', ' 60s', '-1s', 'NaN', '']) {
    assert.throws(() => fuzzArgs(plan.fuzz[0], value));
  }
});

test('selection, modes and matrix overflow fail closed instead of yielding a false green', () => {
  const plan = planChecks(sample);
  assert.equal(selectChecks(plan.checks).length, 3);
  assert.equal(selectChecks(plan.fuzz, './internal/core/annot', 'FuzzParse').length, 1);
  assert.throws(() => selectChecks(plan.fuzz, './internal/core/annot', 'FuzzGone'));
  assert.throws(() => selectChecks(plan.checks, '--help'));
  assert.throws(() => planChecks(sample, 'typo'));
  assert.throws(() => planChecks([pkg('proto', Array.from({ length: 257 }, (_, i) => `Fuzz${i}`))]));
  assert.throws(() => planChecks([pkg('core/new', Array.from({ length: 257 }, (_, i) => `TestProp${i}`))], 'nightly'));
});

test('coverage enforces strictly greater than 90%, weighted by statements, with no empty/malformed escape', () => {
  const profile = (covered, missed) => `mode: atomic\np.go:1.1,2.1 ${covered} 1\np.go:3.1,4.1 ${missed} 0\n`;
  assert.equal(requireCoverage(profile(91, 9)), 91);
  assert(requireCoverage(profile(9001, 999)) > 90); // must not round down to 90.0
  for (const input of [profile(90, 10), profile(1, 99), 'mode: atomic\n', profile(0, 0), 'mode: set\n', 'mode: atomic\nbroken']) {
    assert.throws(() => requireCoverage(input));
  }
});
