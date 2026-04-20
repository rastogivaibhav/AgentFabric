module.exports = {
  root: true,
  env: {
    browser: true,
    es2022: true,
    node: true,
  },
  parser: '@typescript-eslint/parser',
  plugins: ['react-hooks'],
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
    ecmaFeatures: {
      jsx: true,
    },
  },
  ignorePatterns: ['dist/', 'coverage/', 'node_modules/', 'playwright-report/', 'test-results/'],
  rules: {
    'no-empty': 'off',
    'no-redeclare': 'off',
    'no-undef': 'off',
    'no-unused-vars': 'off',
    'react-hooks/rules-of-hooks': 'off',
  },
}
