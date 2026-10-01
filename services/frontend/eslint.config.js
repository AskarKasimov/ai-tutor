import queryPlugin from '@tanstack/eslint-plugin-query'
import tseslint from 'typescript-eslint'

export default [
  {
    ignores: ['dist/', 'src/routeTree.gen.ts'],
  },
  ...tseslint.configs.recommended,
  ...queryPlugin.configs['flat/recommended'],
]
