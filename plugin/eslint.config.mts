import obsidianmd from 'eslint-plugin-obsidianmd';
import globals from 'globals';
import tseslint from 'typescript-eslint';
import { globalIgnores, defineConfig } from 'eslint/config';

export default defineConfig(
	globalIgnores([
		'node_modules',
		'dist',
		'esbuild.config.mjs',
		'version-bump.mjs',
		'versions.json',
		'main.js',
		'package.json',
		'package-lock.json',
		'tsconfig.json',
	]),
	{
		languageOptions: {
			globals: {
				...globals.browser,
			},
			parserOptions: {
				projectService: {
					allowDefaultProject: ['eslint.config.mts', 'manifest.json'],
				},
				tsconfigRootDir: import.meta.dirname,
				extraFileExtensions: ['.json'],
			},
		},
	},
	...obsidianmd.configs.recommended,
	{
		// Existing phase 1 UI wording/API warnings: compatibility debt, ADR 014.
		files: ['src/main.ts', 'src/settings.ts', 'src/ui/StelePullView.ts'],
		rules: { 'obsidianmd/ui/sentence-case': 'off', 'obsidianmd/settings-tab/prefer-setting-definitions': 'off' },
	},
	...tseslint.configs.strictTypeChecked.map(config => ({ ...config, files: ['src/core/**/*.ts'] })),
	{
		files: ['src/core/**/*.ts'],
		rules: {
			'@typescript-eslint/no-explicit-any': 'error',
			'@typescript-eslint/switch-exhaustiveness-check': 'error',
			'no-restricted-imports': ['error', {
				patterns: [{ group: ['obsidian', 'obsidian/*', 'node:*', '../transport/*', '../vault/*', '../ui/*', '**/transport/**', '**/vault/**', '**/ui/**'], message: 'The sync core must be pure.' }],
			}],
			'no-restricted-globals': ['error', 'window', 'document', 'fetch', 'WebSocket', 'localStorage', 'indexedDB'],
			'no-restricted-properties': ['error', { object: 'Date', property: 'now', message: 'Inject time into the core.' }],
		},
	},
	{
		// Tests and build config run under vitest/node on a developer machine
		// and are never bundled into main.js, so the "no Node APIs on mobile"
		// rule does not apply to them.
		files: ['**/*.test.ts', 'test/**/*.ts', 'vitest.config.ts'],
		rules: {
			'obsidianmd/no-nodejs-modules': 'off',
		},
	},
);
