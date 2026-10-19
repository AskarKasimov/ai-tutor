import { AlertDialog, Button, Flex, Select, Text } from '@radix-ui/themes'
import { FileSpreadsheet } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CourseGraphApiError,
  type CourseGraphMapFile,
} from '@/entities/course-graph'
import { useImportCompetencyMapMutation } from '@/features/course-graph-build'
import styles from './competency-map.module.scss'

// Builds the course graph from a competency map instead of the materials:
// either source replaces the graph of the course.
export function CompetencyMapImport({
  userId,
  subjectId,
  hasGraph,
  disabled,
  map,
}: {
  userId: string
  subjectId: string
  hasGraph: boolean
  disabled: boolean
  map: CourseGraphMapFile | null | undefined
}) {
  const { t } = useTranslation()
  const importMap = useImportCompetencyMapMutation(userId, subjectId)
  const input = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [confirming, setConfirming] = useState(false)
  // The sheets to choose from once the server asks for one.
  const [sheets, setSheets] = useState<string[]>([])
  const [sheet, setSheet] = useState<string | null>(null)

  function send(chosen: File, chosenSheet: string | null) {
    importMap.mutate(
      { file: chosen, sheet: chosenSheet },
      {
        onSuccess: () => {
          setFile(null)
          setSheets([])
          setSheet(null)
        },
        onError: (error) => {
          if (
            error instanceof CourseGraphApiError &&
            error.code === 'COMPETENCY_MAP_SHEET_REQUIRED'
          ) {
            const names = error.details
              .filter((detail) => detail.path === 'sheet')
              .map((detail) => detail.message)
            setSheets(names)
            setSheet(names[0] ?? null)
          }
        },
      },
    )
  }

  const error = importMap.error
  const cellIssues =
    error instanceof CourseGraphApiError
      ? error.details.filter((detail) => detail.path === 'file')
      : []
  // The sheets stay offered until the import succeeds or another file is
  // chosen, so a failed sheet can be retried or another one chosen.
  const askingSheet = file !== null && sheets.length > 0
  const sheetRequired =
    error instanceof CourseGraphApiError &&
    error.code === 'COMPETENCY_MAP_SHEET_REQUIRED'

  return (
    <div className={styles.mapImport}>
      <Text as="p" color="gray" size="2">
        {t('courseGraph.mapImportHint')}
      </Text>
      {map && (
        <Text as="p" size="2">
          {t('courseGraph.mapImported', {
            filename: map.filename,
            sheet: map.sheet,
          })}
        </Text>
      )}
      <input
        ref={input}
        type="file"
        accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        hidden
        aria-label={t('courseGraph.mapImport')}
        onChange={(event) => {
          const chosen = event.target.files?.[0] ?? null
          event.target.value = ''
          if (!chosen) return
          importMap.reset()
          setSheets([])
          setSheet(null)
          setFile(chosen)
          setConfirming(true)
        }}
      />
      <Flex gap="3" align="center" wrap="wrap">
        <Button
          size="3"
          variant="soft"
          disabled={disabled || importMap.isPending}
          onClick={() => input.current?.click()}
        >
          <FileSpreadsheet size={17} aria-hidden="true" />
          {importMap.isPending
            ? t('courseGraph.mapImporting')
            : t('courseGraph.mapImport')}
        </Button>
      </Flex>
      {askingSheet && (
        <Flex gap="3" align="end" wrap="wrap" role="group">
          <label className={styles.mapSheetField}>
            <Text as="span" size="2">
              {t('courseGraph.mapSheet')}
            </Text>
            <Select.Root
              size="3"
              value={sheet ?? undefined}
              disabled={importMap.isPending}
              onValueChange={setSheet}
            >
              <Select.Trigger aria-label={t('courseGraph.mapSheet')} />
              <Select.Content>
                {sheets.map((name) => (
                  <Select.Item key={name} value={name}>
                    {name}
                  </Select.Item>
                ))}
              </Select.Content>
            </Select.Root>
          </label>
          <Button
            size="3"
            color="green"
            disabled={disabled || importMap.isPending || !sheet}
            onClick={() => send(file, sheet)}
          >
            {t('courseGraph.mapImportSheet')}
          </Button>
        </Flex>
      )}
      {error && !sheetRequired && (
        <div className={styles.error} role="alert">
          <Text weight="bold">
            {error instanceof Error
              ? error.message
              : t('courseGraph.mapImportFailed')}
          </Text>
          {cellIssues.length > 0 && (
            <ul>
              {cellIssues.map((issue) => (
                <li key={issue.message}>{issue.message}</li>
              ))}
            </ul>
          )}
        </div>
      )}
      <AlertDialog.Root open={confirming} onOpenChange={setConfirming}>
        <AlertDialog.Content maxWidth="440px">
          <AlertDialog.Title>
            {t('courseGraph.mapImportTitle')}
          </AlertDialog.Title>
          <AlertDialog.Description>
            {t(
              hasGraph
                ? 'courseGraph.mapImportReplace'
                : 'courseGraph.mapImportConfirm',
              { filename: file?.name ?? '' },
            )}
          </AlertDialog.Description>
          <Flex gap="3" mt="4" justify="end">
            <AlertDialog.Cancel>
              <Button variant="soft" color="gray">
                {t('competencyMap.cancel')}
              </Button>
            </AlertDialog.Cancel>
            <AlertDialog.Action>
              <Button
                color="green"
                disabled={!file || disabled}
                onClick={() => file && send(file, null)}
              >
                {t('courseGraph.mapImportStart')}
              </Button>
            </AlertDialog.Action>
          </Flex>
        </AlertDialog.Content>
      </AlertDialog.Root>
    </div>
  )
}
