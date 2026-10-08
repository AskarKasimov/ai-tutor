import ts from 'typescript'
import { describe, expect, it } from 'vitest'

const modules = import.meta.glob('../**/*.{ts,tsx}', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>
const apiNames = new Set([
  'readCurrentUser',
  'authenticate',
  'logout',
  'readCompetencyMap',
  'importCompetencyMap',
  'createVariant',
  'startDiagnostic',
  'readDiagnostic',
  'submitDiagnostic',
  'diagnosticAudio',
  'readDiagnosticResult',
  'transcribeRecording',
  'synthesizeQuestion',
  'evaluateAnswer',
  'apiFetch',
])
describe('network ownership boundaries', () => {
  it('keeps native fetch in the transport and API functions out of routes/platform/orchestration', () => {
    const violations: string[] = []
    for (const [file, text] of Object.entries(modules)) {
      const path = file.startsWith('./')
        ? `data/${file.slice(2)}`
        : file.replace(/^\.\.\//, '')
      if (/\.test\.[cm]?[jt]sx?$/.test(path)) continue
      const source = ts.createSourceFile(
        path,
        text,
        ts.ScriptTarget.Latest,
        true,
        file.endsWith('x') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
      )
      const restricted =
        path.startsWith('routes/') ||
        path.startsWith('platform/') ||
        [
          'data/use-trainer-prototype.ts',
          'data/use-diagnostic-voice.ts',
        ].includes(path)
      function visit(node: ts.Node) {
        if (
          ts.isCallExpression(node) &&
          ts.isIdentifier(node.expression) &&
          node.expression.text === 'fetch' &&
          path !== 'data/api-fetch.ts'
        )
          violations.push(`${path}: direct fetch`)
        if (restricted && ts.isImportDeclaration(node)) {
          const moduleName = ts.isStringLiteral(node.moduleSpecifier)
            ? node.moduleSpecifier.text
            : ''
          const bindings = node.importClause?.namedBindings
          if (
            moduleName.startsWith('../data/') ||
            moduleName.startsWith('./')
          ) {
            if (bindings && ts.isNamedImports(bindings))
              for (const item of bindings.elements) {
                const name = item.propertyName?.text ?? item.name.text
                if (
                  apiNames.has(name) &&
                  !(
                    path === 'data/use-diagnostic-voice.ts' &&
                    name === 'createSubmission'
                  )
                )
                  violations.push(`${path}: imports ${name} from ${moduleName}`)
              }
            if (
              bindings &&
              ts.isNamespaceImport(bindings) &&
              /(?:api|fetch)/i.test(moduleName)
            )
              violations.push(`${path}: namespace import ${moduleName}`)
          }
        }
        ts.forEachChild(node, visit)
      }
      visit(source)
    }
    expect(violations).toEqual([])
  })
})
