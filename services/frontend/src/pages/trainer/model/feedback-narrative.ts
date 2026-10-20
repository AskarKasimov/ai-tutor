// A completed review explains the answer; it must not turn into a new task.
// Apply at presentation time too, so historical feedback is readable immediately.
const request =
  /(?:^|[^\p{L}])(?:давайте|добавьте|дополните|сформулируйте|опишите|проанализируйте|перечислите|вспомните|запишите|повторите|объясните|назовите|выберите|сравните|приведите|расскажите|ответьте|уточните|проверьте|определите|изучите|пересмотрите|разберите|закрепите|попробуйте|обратите|укажите|продолжите|продолжайте|выполните|подумайте|обоснуйте|посчитайте|рассчитайте|докажите|решите|отправьте|перейдите)(?=$|[^\p{L}])|(?:нужно|необходимо|следует|стоит|можете)\s+(?:вам\s+)?(?:добавить|дополнить|сформулировать|описать|перечислить|записать|указать|определить|сравнить|пояснить|рассказать|уточнить|объяснить|ответить|назвать|выбрать|обосновать|привести|решить|посчитать)(?=$|[^\p{L}])/iu

export function feedbackNarrative(text: string) {
  let fence: { marker: string; length: number } | undefined
  return text
    .split('\n')
    .map((line) => {
      const marker = /^ {0,3}(`{3,}|~{3,})/.exec(line)
      if (fence) {
        if (
          marker &&
          marker[1][0] === fence.marker &&
          marker[1].length >= fence.length &&
          !line.slice(marker[0].length).trim()
        ) {
          fence = undefined
        }
        return line
      }
      if (marker) {
        fence = { marker: marker[1][0], length: marker[1].length }
        return line
      }
      if (/^(?: {4}|\t)/.test(line)) return line
      const inlineCode: string[] = []
      const masked = line.replace(/(`+)(.+?)\1/g, (code) => {
        inlineCode.push(code)
        return `\uE000${inlineCode.length - 1}\uE001`
      })
      const sentences =
        masked.match(/.+?(?:[.!?…]+(?:[*_»")]+)?(?=\s|$)|$)/gu) ?? []
      return sentences
        .filter((sentence) => {
          const prose = sentence.replace(/`[^`]*`/g, '')
          return !prose.includes('?') && !request.test(prose)
        })
        .map((sentence) => sentence.trim())
        .join(' ')
        .replace(
          /\uE000(\d+)\uE001/g,
          (_, index: string) => inlineCode[Number(index)],
        )
    })
    .join('\n')
    .replace(/^\n+|\n+$/g, '')
}
