import ts from 'typescript'
import { describe, expect, it } from 'vitest'

const modules = import.meta.glob('/src/**/*.{ts,tsx}', {
  eager: true,
  query: '?raw',
  import: 'default',
}) as Record<string, string>

function sourcePath(file: string) {
  return file.replace(/^\/?src\//, '')
}

function resolveImport(from: string, specifier: string): string | null {
  let base: string[]
  if (specifier.startsWith('@/')) base = specifier.slice(2).split('/')
  else if (specifier.startsWith('.'))
    base = [...from.split('/').slice(0, -1), ...specifier.split('/')]
  else return null
  const resolved: string[] = []
  for (const part of base) {
    if (!part || part === '.') continue
    if (part === '..') resolved.pop()
    else resolved.push(part)
  }
  return resolved.join('/')
}

function layerAndSlice(path: string) {
  const [layer, slice] = path.split('/')
  return { layer, slice }
}

const order: Record<string, number> = {
  bootstrap: 0,
  pages: 1,
  widgets: 2,
  features: 3,
  entities: 4,
  shared: 5,
}

describe('FSD boundaries', () => {
  it('scans all source files and the expected FSD slices', () => {
    const paths = Object.keys(modules).map(sourcePath)
    expect(paths).toEqual(
      expect.arrayContaining([
        'bootstrap/providers.tsx',
        'pages/trainer/ui/trainer-screen.tsx',
        'features/diagnostic-session/model/start-session.ts',
        'features/voice-answer/model/use-trainer-voice.ts',
        'entities/user/model/user.ts',
        'entities/assessment/model/feedback.ts',
        'shared/api/api-fetch.ts',
        'routes/index.tsx',
      ]),
    )
    expect(
      paths.some((path) =>
        /^(application|domain|data|infrastructure|platform)\//.test(path),
      ),
    ).toBe(false)
    expect(
      paths.some(
        (path) => /\.(test|spec)\.tsx?$/.test(path) || path.startsWith('test/'),
      ),
    ).toBe(false)
  })

  it('enforces layer order, slice isolation, and public APIs for cross-slice imports', () => {
    const violations: string[] = []
    for (const [file, text] of Object.entries(modules)) {
      const current = sourcePath(file)
      if (
        /\.test\.[cm]?[jt]sx?$/.test(current) ||
        current === 'routeTree.gen.ts'
      )
        continue
      const parsed = ts.createSourceFile(
        current,
        text,
        ts.ScriptTarget.Latest,
        true,
        current.endsWith('x') ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
      )
      const { layer: fromLayer, slice: fromSlice } = layerAndSlice(current)
      function inspect(specifier: string) {
        const target = resolveImport(current, specifier)
        if (!target) return
        const { layer: toLayer, slice: toSlice } = layerAndSlice(target)
        if (!order.hasOwnProperty(toLayer)) return
        if (fromLayer === 'routes') {
          if (toLayer !== 'bootstrap' && toLayer !== 'pages')
            violations.push(`${current}: route imports ${target}`)
          if (toLayer === 'pages' && !isPublicTarget(target, toLayer, toSlice))
            violations.push(
              `${current}: page import bypasses public API ${specifier}`,
            )
          return
        }
        if (!order.hasOwnProperty(fromLayer)) return
        if (order[toLayer] <= order[fromLayer]) {
          if (toLayer !== fromLayer)
            violations.push(`${current}: imports upward ${target}`)
          else if (
            !['bootstrap', 'shared'].includes(fromLayer) &&
            toSlice !== fromSlice
          ) {
            const crossEntity =
              fromLayer === 'entities' &&
              target === `entities/${toSlice}/@x/${fromSlice}`
            if (!crossEntity)
              violations.push(`${current}: same-layer slice import ${target}`)
          }
        }
        if (
          fromLayer === 'shared' &&
          toLayer === 'shared' &&
          fromSlice !== toSlice &&
          !isPublicTarget(target, toLayer, toSlice)
        )
          violations.push(
            `${current}: shared segment bypasses public API ${specifier}`,
          )
        if (toLayer !== fromLayer) {
          const crossEntity =
            fromLayer === 'entities' &&
            target === `entities/${toSlice}/@x/${fromSlice}`
          if (!crossEntity && !isPublicTarget(target, toLayer, toSlice))
            violations.push(`${current}: public API bypass ${specifier}`)
        }
      }
      function isPublicTarget(target: string, layer: string, slice: string) {
        return (
          target === `${layer}/${slice}` ||
          target === `${layer}/${slice}/index` ||
          target === `${layer}/${slice}/index.ts` ||
          target === `${layer}/${slice}/index.tsx`
        )
      }
      function visit(node: ts.Node): void {
        if (
          ts.isImportDeclaration(node) &&
          ts.isStringLiteral(node.moduleSpecifier)
        )
          inspect(node.moduleSpecifier.text)
        if (
          ts.isExportDeclaration(node) &&
          node.moduleSpecifier &&
          ts.isStringLiteral(node.moduleSpecifier)
        )
          inspect(node.moduleSpecifier.text)
        if (
          ts.isCallExpression(node) &&
          node.expression.kind === ts.SyntaxKind.ImportKeyword &&
          node.arguments[0] &&
          ts.isStringLiteral(node.arguments[0])
        )
          inspect(node.arguments[0].text)
        if (
          ts.isCallExpression(node) &&
          ts.isIdentifier(node.expression) &&
          node.expression.text === 'fetch' &&
          current !== 'shared/api/api-fetch.ts'
        )
          violations.push(`${current}: direct fetch`)
        ts.forEachChild(node, visit)
      }
      visit(parsed)
    }
    expect(violations).toEqual([])
  })

  it('keeps independent diagnostic orchestration and feedback parsing free of runtime packages and HTTP', () => {
    const pure = [
      'features/diagnostic-session/model/start-session.ts',
      'features/diagnostic-session/model/bootstrap-coordinator.ts',
      'entities/assessment/model/feedback.ts',
    ]
    const violations: string[] = []
    for (const path of pure) {
      const source = modules[`/src/${path}`]
      if (typeof source !== 'string') {
        violations.push(`missing ${path}`)
        continue
      }
      const parsed = ts.createSourceFile(
        path,
        source,
        ts.ScriptTarget.Latest,
        true,
        ts.ScriptKind.TS,
      )
      function visit(node: ts.Node) {
        if (
          ts.isImportDeclaration(node) &&
          ts.isStringLiteral(node.moduleSpecifier)
        ) {
          const clause = node.importClause
          const namedImports = clause?.namedBindings
          const onlyTypeBindings =
            !!namedImports &&
            ts.isNamedImports(namedImports) &&
            namedImports.elements.length > 0 &&
            namedImports.elements.every((element) => element.isTypeOnly)
          const isTypeOnly = clause?.isTypeOnly || onlyTypeBindings
          if (!isTypeOnly && !node.moduleSpecifier.text.startsWith('.'))
            violations.push(
              `${path}: runtime package or HTTP dependency ${node.moduleSpecifier.text}`,
            )
        }
        if (
          ts.isExportDeclaration(node) &&
          node.moduleSpecifier &&
          ts.isStringLiteral(node.moduleSpecifier) &&
          !node.isTypeOnly &&
          !node.moduleSpecifier.text.startsWith('.')
        )
          violations.push(
            `${path}: runtime re-export dependency ${node.moduleSpecifier.text}`,
          )
        if (
          ts.isCallExpression(node) &&
          node.expression.kind === ts.SyntaxKind.ImportKeyword &&
          node.arguments[0] &&
          ts.isStringLiteral(node.arguments[0]) &&
          !node.arguments[0].text.startsWith('.')
        )
          violations.push(
            `${path}: runtime dynamic dependency ${node.arguments[0].text}`,
          )
        if (
          ts.isCallExpression(node) &&
          ts.isIdentifier(node.expression) &&
          node.expression.text === 'fetch'
        )
          violations.push(`${path}: direct fetch`)
        ts.forEachChild(node, visit)
      }
      visit(parsed)
    }
    expect(violations).toEqual([])
  })
})
