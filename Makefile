.PHONY: tools lint test test-integration fuzz-short sim fixtures fixtures-check oracle-scale oracle-replay textop-acceptance textop-perf annot-acceptance contract plugin audit ci dev-up dev-down
# Each recipe is also a command that works directly in PowerShell.
tools:
	node tools/tasks.mjs tools
lint:
	node tools/tasks.mjs lint
test:
	node tools/tasks.mjs test
test-integration:
	node tools/tasks.mjs test-integration
fuzz-short:
	node tools/tasks.mjs fuzz-short
sim:
	node tools/tasks.mjs sim
fixtures:
	node tools/tasks.mjs fixtures
fixtures-check:
	node tools/tasks.mjs fixtures-check
oracle-scale:
	node tools/tasks.mjs oracle-scale
oracle-replay:
	node tools/tasks.mjs oracle-replay
textop-acceptance:
	node tools/tasks.mjs textop-acceptance
textop-perf:
	node tools/tasks.mjs textop-perf
annot-acceptance:
	node tools/tasks.mjs annot-acceptance
contract:
	node tools/tasks.mjs contract
plugin:
	node tools/tasks.mjs plugin
audit:
	node tools/tasks.mjs audit
ci:
	node tools/tasks.mjs ci
dev-up:
	node tools/dev.mjs up
dev-down:
	docker compose down
