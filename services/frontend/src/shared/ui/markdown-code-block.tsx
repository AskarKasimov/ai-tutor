import { isValidElement, type ComponentProps, type ReactNode } from 'react'
import { Light as SyntaxHighlighter } from 'react-syntax-highlighter'
import bash from 'react-syntax-highlighter/dist/esm/languages/hljs/bash'
import cpp from 'react-syntax-highlighter/dist/esm/languages/hljs/cpp'
import css from 'react-syntax-highlighter/dist/esm/languages/hljs/css'
import go from 'react-syntax-highlighter/dist/esm/languages/hljs/go'
import java from 'react-syntax-highlighter/dist/esm/languages/hljs/java'
import javascript from 'react-syntax-highlighter/dist/esm/languages/hljs/javascript'
import json from 'react-syntax-highlighter/dist/esm/languages/hljs/json'
import python from 'react-syntax-highlighter/dist/esm/languages/hljs/python'
import rust from 'react-syntax-highlighter/dist/esm/languages/hljs/rust'
import sql from 'react-syntax-highlighter/dist/esm/languages/hljs/sql'
import typescript from 'react-syntax-highlighter/dist/esm/languages/hljs/typescript'
import xml from 'react-syntax-highlighter/dist/esm/languages/hljs/xml'

const languages = {
  bash,
  cpp,
  css,
  go,
  java,
  javascript,
  json,
  python,
  rust,
  sql,
  typescript,
  xml,
}
for (const [name, grammar] of Object.entries(languages)) {
  SyntaxHighlighter.registerLanguage(name, grammar)
}
const aliases: Record<string, string> = {
  py: 'python',
  js: 'javascript',
  jsx: 'javascript',
  ts: 'typescript',
  tsx: 'typescript',
  sh: 'bash',
  shell: 'bash',
  html: 'xml',
  svg: 'xml',
  c: 'cpp',
  'c++': 'cpp',
  rs: 'rust',
}

// The library adds a white-space inline style even with useInlineStyles=false.
// Keep that presentation in the Markdown SCSS alongside the theme colors.
function CodeTag({ className, children }: ComponentProps<'code'>) {
  return <code className={className}>{children}</code>
}

export function MarkdownCodeBlock({ children }: { children?: ReactNode }) {
  if (
    !isValidElement<{ className?: string; children?: ReactNode }>(children) ||
    typeof children.props.children !== 'string'
  ) {
    return <pre>{children}</pre>
  }
  const label = /(?:^|\s)language-(\S+)/
    .exec(children.props.className ?? '')?.[1]
    .toLowerCase()
  const language = label ? (aliases[label] ?? label) : undefined
  if (!language || !Object.hasOwn(languages, language)) {
    return <pre>{children}</pre>
  }
  return (
    <SyntaxHighlighter
      language={language}
      useInlineStyles={false}
      CodeTag={CodeTag}
    >
      {children.props.children}
    </SyntaxHighlighter>
  )
}
