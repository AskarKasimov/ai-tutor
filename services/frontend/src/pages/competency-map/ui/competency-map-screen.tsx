import {
  AlertDialog,
  Button,
  Flex,
  Heading,
  Table,
  Text,
  TextField,
} from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import {
  Check,
  CheckCircle2,
  FileSpreadsheet,
  Clock,
  History,
  Layers,
  ListChecks,
  Shapes,
  ShieldCheck,
  Target,
  TriangleAlert,
  Upload,
  X,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CompetencyMapApiError } from '@/entities/competency-map'
import { useAuth } from '@/features/auth'
import {
  useCompetencyMapQuery,
  useImportCompetencyMapMutation,
} from '@/features/import-competency-map'
import type { CompetencyMapSummary } from '@/entities/competency-map'
import { AccountMenu } from '@/features/auth'
import {
  useCreateSubjectMutation,
  useSubjectsQuery,
} from '@/features/subject-selection'
import styles from '@/pages/competency-map/ui/competency-map.module.scss'

export function CompetencyMapAdminScreen() {
  const { t } = useTranslation()
  const { user } = useAuth()
  const navigate = useNavigate()
  if (user?.role !== 'admin')
    return (
      <main className={styles.denied}>
        <Heading as="h1">{t('competencyMap.title')}</Heading>
        <Text as="p" role="alert">
          {t('competencyMap.forbidden')}
        </Text>
        <Button onClick={() => void navigate({ to: '/' })}>
          {t('competencyMap.toLesson')}
        </Button>
      </main>
    )
  return <CompetencyMapAdmin userId={user.id} />
}

function CompetencyMapAdmin({ userId }: { userId: string }) {
  const { t } = useTranslation()
  const subjects = useSubjectsQuery(userId)
  const create = useCreateSubjectMutation(userId)
  // The teacher manages one subject; a new one is used until the list refetches.
  const [createdId, setCreatedId] = useState('')
  const subject = subjects.data?.[0]
  const subjectId = subject?.id ?? createdId
  const query = useCompetencyMapQuery(userId, subjectId)
  const upload = useImportCompetencyMapMutation(userId, subjectId)
  const [subjectName, setSubjectName] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [queuedImport, setQueuedImport] = useState(false)

  // The import hook aborts on subject change, so the first import starts only
  // after the created subject ID is committed.
  useEffect(() => {
    if (!queuedImport || !subjectId || !file) return
    setQueuedImport(false)
    upload.mutate({ file, subjectId }, { onSuccess: () => setFile(null) })
  }, [queuedImport, subjectId, file, upload])

  function selectFile(selected: File | undefined) {
    upload.reset()
    setFile(null)
    setFileError(null)
    if (!selected) return
    if (!/\.(csv|xlsx)$/i.test(selected.name)) {
      setFileError(t('competencyMap.invalidType'))
      return
    }
    if (selected.size > 25 * 1024 * 1024) {
      setFileError(t('competencyMap.tooLarge'))
      return
    }
    if (selected.size === 0) {
      setFileError(t('competencyMap.emptyFile'))
      return
    }
    setFile(selected)
  }

  const busy = create.isPending || upload.isPending || queuedImport
  const error = upload.error
  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <ShieldCheck size={24} aria-hidden="true" />
          <Text weight="bold">{t('competencyMap.admin')}</Text>
        </div>
        <AccountMenu />
      </header>
      <main className={styles.content}>
        <div className={styles.intro}>
          <Heading as="h1" size="8">
            {t('competencyMap.title')}
          </Heading>
          <Text as="p" color="gray">
            {t(
              subjectId
                ? 'competencyMap.editSubtitle'
                : 'competencyMap.subtitle',
            )}
          </Text>
        </div>
        {subjects.isPending ? (
          <Text as="p" role="status">
            {t('competencyMap.subjectsLoading')}
          </Text>
        ) : subjects.isError ? (
          <div className={styles.card}>
            <Text as="p" role="alert">
              {t('competencyMap.subjectsError')}
            </Text>
            <Button variant="soft" onClick={() => void subjects.refetch()}>
              {t('trainer.retry')}
            </Button>
          </div>
        ) : !subjectId ? (
          <section className={styles.card} aria-labelledby="subject-form-title">
            <Heading as="h2" size="5" id="subject-form-title">
              {t('competencyMap.createSubject')}
            </Heading>
            <TextField.Root
              size="3"
              aria-label={t('competencyMap.subjectName')}
              value={subjectName}
              placeholder={t('competencyMap.subjectPlaceholder')}
              disabled={busy}
              onChange={(event) => setSubjectName(event.target.value)}
            />
            <MapFileZone file={file} disabled={busy} onSelect={selectFile} />
            {fileError && (
              <Text as="p" role="alert" color="red" size="2">
                {fileError}
              </Text>
            )}
            {create.isError && (
              <Text as="p" role="alert" color="red" size="2">
                {t('competencyMap.createSubjectError')}
              </Text>
            )}
            <Button
              size="3"
              color="green"
              className={styles.submit}
              disabled={!subjectName.trim() || !file || busy}
              onClick={() => {
                const name = subjectName.trim()
                if (!name || !file) return
                create.mutate(name, {
                  onSuccess: (created) => {
                    setCreatedId(created.id)
                    setQueuedImport(true)
                  },
                })
              }}
            >
              <Check size={18} aria-hidden="true" />
              {t(busy ? 'competencyMap.uploading' : 'competencyMap.submit')}
            </Button>
          </section>
        ) : (
          <>
            <section className={styles.card} aria-labelledby="subject-title">
              <div className={styles.editHeading}>
                <Text as="p" size="2" weight="medium" color="gray">
                  {t('competencyMap.subject')}
                </Text>
                <Heading as="h2" size="6" id="subject-title">
                  {subject?.name ?? subjectName.trim()}
                </Heading>
              </div>
              {query.isPending ? (
                <Text as="p" role="status" color="gray" size="2">
                  {t('competencyMap.loading')}
                </Text>
              ) : query.isError ? (
                <Flex gap="3" align="center" wrap="wrap">
                  <Text as="p" role="alert" color="red" size="2">
                    {t('competencyMap.loadError')}
                  </Text>
                  <Button
                    variant="ghost"
                    size="1"
                    onClick={() => void query.refetch()}
                  >
                    {t('trainer.retry')}
                  </Button>
                </Flex>
              ) : query.data.importedAt === null ? (
                <Text as="p" color="gray" size="2">
                  {t('competencyMap.empty')}
                </Text>
              ) : (
                <>
                  <Text as="p" role="status" className={styles.success}>
                    <CheckCircle2 size={20} aria-hidden="true" />
                    {t('competencyMap.success')}
                  </Text>
                  <MapSummary summary={query.data} />
                </>
              )}
            </section>
            <section
              className={styles.card}
              aria-labelledby="subject-form-title"
            >
              <Heading as="h2" size="4" id="subject-form-title">
                {t('competencyMap.edit')}
              </Heading>
              <MapFileZone file={file} disabled={busy} onSelect={selectFile} />
              {fileError && (
                <Text as="p" role="alert" color="red" size="2">
                  {fileError}
                </Text>
              )}
              {busy && (
                <Text as="p" role="status" size="2">
                  {t('competencyMap.uploading')}
                </Text>
              )}
              {error && (
                <div role="alert" className={styles.error}>
                  <Text as="p">
                    {error instanceof CompetencyMapApiError
                      ? error.message
                      : t('competencyMap.unknownResult')}
                  </Text>
                  {error instanceof CompetencyMapApiError &&
                    error.details.length > 0 && (
                      <ul>
                        {error.details.map((detail, index) => (
                          <li key={index}>
                            {detail.path?.startsWith('row:')
                              ? t('competencyMap.rowError', {
                                  row: detail.path.slice(4),
                                  message: detail.message,
                                })
                              : detail.message}
                          </li>
                        ))}
                      </ul>
                    )}
                </div>
              )}
              <div className={styles.actions}>
                <AlertDialog.Root>
                  <AlertDialog.Trigger>
                    <Button
                      size="3"
                      color="green"
                      className={styles.submit}
                      disabled={!file || busy}
                    >
                      <Check size={18} aria-hidden="true" />
                      {t('competencyMap.submitEdit')}
                    </Button>
                  </AlertDialog.Trigger>
                  <AlertDialog.Content maxWidth="440px">
                    <AlertDialog.Title>
                      {t('competencyMap.confirmTitle')}
                    </AlertDialog.Title>
                    <AlertDialog.Description>
                      {t('competencyMap.confirmHelp')}
                    </AlertDialog.Description>
                    <Flex gap="3" mt="5" justify="end" wrap="wrap">
                      <AlertDialog.Cancel>
                        <Button variant="soft" color="gray">
                          {t('competencyMap.cancel')}
                        </Button>
                      </AlertDialog.Cancel>
                      <AlertDialog.Action>
                        <Button
                          color="red"
                          onClick={() => {
                            if (file && !busy)
                              upload.mutate(
                                { file, subjectId },
                                { onSuccess: () => setFile(null) },
                              )
                          }}
                        >
                          {t('competencyMap.confirm')}
                        </Button>
                      </AlertDialog.Action>
                    </Flex>
                  </AlertDialog.Content>
                </AlertDialog.Root>
                {file && !busy && (
                  <Button
                    size="3"
                    color="red"
                    variant="soft"
                    className={styles.submit}
                    onClick={() => selectFile(undefined)}
                  >
                    <X size={18} aria-hidden="true" />
                    {t('competencyMap.cancelEdit')}
                  </Button>
                )}
              </div>
            </section>
          </>
        )}
        {upload.data &&
          upload.variables?.subjectId === subjectId &&
          (upload.data.unparsedTaskCellCount > 0 ||
            upload.data.warnings.length > 0) && (
            <section
              className={styles.card}
              aria-labelledby="import-warnings-title"
            >
              <div className={styles.sectionHeading} data-tone="warning">
                <TriangleAlert aria-hidden="true" />
                <Heading as="h2" size="4" id="import-warnings-title">
                  {t('competencyMap.warnings')}
                </Heading>
              </div>
              {upload.data.unparsedTaskCellCount > 0 && (
                <Text as="p" color="amber" size="2">
                  {t('competencyMap.unparsed', {
                    count: upload.data.unparsedTaskCellCount,
                  })}
                </Text>
              )}
              {upload.data.warnings.length > 0 && (
                <>
                  <div className={styles.tableScroll}>
                    <Table.Root variant="surface">
                      <Table.Header>
                        <Table.Row>
                          <Table.ColumnHeaderCell>
                            {t('competencyMap.row')}
                          </Table.ColumnHeaderCell>
                          <Table.ColumnHeaderCell>
                            {t('competencyMap.column')}
                          </Table.ColumnHeaderCell>
                          <Table.ColumnHeaderCell>
                            {t('competencyMap.reason')}
                          </Table.ColumnHeaderCell>
                        </Table.Row>
                      </Table.Header>
                      <Table.Body>
                        {upload.data.warnings.map((warning, index) => (
                          <Table.Row key={index}>
                            <Table.RowHeaderCell>
                              {warning.row}
                            </Table.RowHeaderCell>
                            <Table.Cell>{warning.column}</Table.Cell>
                            <Table.Cell>
                              {t(`competencyMap.warning.${warning.code}`, {
                                defaultValue: t(
                                  'competencyMap.warning.unknown',
                                  {
                                    code: warning.code,
                                  },
                                ),
                              })}
                            </Table.Cell>
                          </Table.Row>
                        ))}
                      </Table.Body>
                    </Table.Root>
                  </div>
                </>
              )}
            </section>
          )}
      </main>
    </div>
  )
}

const summaryRows = [
  { key: 'competencyCount', icon: Layers, tone: 'blue' },
  { key: 'constituentCount', icon: Shapes, tone: 'violet' },
  { key: 'outcomeCount', icon: Target, tone: 'green' },
  { key: 'taskCount', icon: ListChecks, tone: 'amber' },
] as const

function MapSummary({ summary }: { summary: CompetencyMapSummary }) {
  const { t, i18n } = useTranslation()
  return (
    <dl className={styles.stats}>
      {summaryRows.map(({ key, icon: Icon, tone }) => (
        <div key={key} className={styles.statRow} data-tone={tone}>
          <span className={styles.statIcon} aria-hidden="true">
            <Icon size={16} />
          </span>
          <dt>{t(`competencyMap.stats.${key}`)}</dt>
          <dd>{summary[key].toLocaleString(i18n.language)}</dd>
        </div>
      ))}
      {summary.importedAt !== null && (
        <div className={styles.statRow} data-tone="gray">
          <span className={styles.statIcon} aria-hidden="true">
            <Clock size={16} />
          </span>
          <dt>{t('competencyMap.stats.importedAt')}</dt>
          <dd>
            {/* Date and time without the locale's separating comma. */}
            {new Date(summary.importedAt * 1000).toLocaleDateString(
              i18n.language,
              { dateStyle: 'medium' },
            )}{' '}
            {new Date(summary.importedAt * 1000).toLocaleTimeString(
              i18n.language,
              { timeStyle: 'short' },
            )}
          </dd>
        </div>
      )}
      <div className={styles.statRow} data-tone="gray">
        <span className={styles.statIcon} aria-hidden="true">
          <History size={16} />
        </span>
        <dt>{t('competencyMap.stats.revision')}</dt>
        <dd>{summary.revision}</dd>
      </div>
    </dl>
  )
}

function MapFileZone({
  file,
  disabled,
  onSelect,
}: {
  file: File | null
  disabled: boolean
  onSelect: (file: File | undefined) => void
}) {
  const { t, i18n } = useTranslation()
  const [dragging, setDragging] = useState(false)
  return (
    <label
      className={styles.dropzone}
      data-dragging={dragging || undefined}
      data-selected={file ? true : undefined}
      data-disabled={disabled || undefined}
      onDragOver={(event) => {
        event.preventDefault()
        if (!disabled) setDragging(true)
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(event) => {
        event.preventDefault()
        setDragging(false)
        if (!disabled) onSelect(event.dataTransfer.files[0])
      }}
    >
      <input
        type="file"
        className={styles.fileInput}
        aria-label={t('competencyMap.file')}
        accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        disabled={disabled}
        onChange={(event) => {
          onSelect(event.target.files?.[0])
          // Allow choosing the same file again after an error.
          event.target.value = ''
        }}
      />
      <span className={styles.dropzoneIcon} aria-hidden="true">
        {file ? <FileSpreadsheet size={22} /> : <Upload size={22} />}
      </span>
      <span className={styles.dropzoneText}>
        <Text weight="bold">
          {file ? file.name : t('competencyMap.dropTitle')}
        </Text>
        <Text size="2" color="gray">
          {file
            ? `${t('competencyMap.fileSize', {
                size: (file.size / (1024 * 1024)).toLocaleString(
                  i18n.language,
                  { maximumFractionDigits: 2 },
                ),
              })} · ${t('competencyMap.dropReplace')}`
            : t('competencyMap.dropHint')}
        </Text>
      </span>
    </label>
  )
}
