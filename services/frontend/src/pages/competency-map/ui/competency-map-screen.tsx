import {
  AlertDialog,
  Badge,
  Button,
  Callout,
  Flex,
  Heading,
  Table,
  Text,
  TextField,
} from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import {
  CheckCircle2,
  FileSpreadsheet,
  ShieldCheck,
  Upload,
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
import type { Subject } from '@/entities/subject'
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
  const { t, i18n } = useTranslation()
  const subjects = useSubjectsQuery(userId)
  const create = useCreateSubjectMutation(userId)
  const [subjectId, setSubjectId] = useState('')
  const [subjectName, setSubjectName] = useState('')
  const query = useCompetencyMapQuery(userId, subjectId)
  const upload = useImportCompetencyMapMutation(userId, subjectId)
  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const selectedSubject = subjects.data?.find(
    (subject) => subject.id === subjectId,
  )

  useEffect(() => {
    if (
      subjects.data &&
      !subjects.data.some((subject) => subject.id === subjectId)
    )
      setSubjectId(subjects.data[0]?.id ?? '')
  }, [subjects.data, subjectId])

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
          <Badge size="2">{t('competencyMap.admin')}</Badge>
          <Heading as="h1" size="8">
            {t('competencyMap.title')}
          </Heading>
          <Text as="p" color="gray">
            {t('competencyMap.subtitle')}
          </Text>
        </div>
        <section className={styles.card} aria-labelledby="current-map-title">
          <Heading as="h2" size="4" id="current-map-title">
            {t('competencyMap.current')}
          </Heading>
          {subjects.isPending ? (
            <Text as="p" role="status">
              {t('competencyMap.subjectsLoading')}
            </Text>
          ) : subjects.isError ? (
            <div>
              <Text as="p" role="alert">
                {t('competencyMap.subjectsError')}
              </Text>
              <Button variant="soft" onClick={() => void subjects.refetch()}>
                {t('trainer.retry')}
              </Button>
            </div>
          ) : (
            <>
              <label htmlFor="map-subject">{t('competencyMap.subject')}</label>
              <select
                id="map-subject"
                value={subjectId}
                onChange={(event) => {
                  setSubjectId(event.target.value)
                  upload.reset()
                  setFile(null)
                  setFileError(null)
                }}
              >
                {subjects.data?.map((subject: Subject) => (
                  <option key={subject.id} value={subject.id}>
                    {subject.name}
                  </option>
                ))}
              </select>
            </>
          )}
          {subjects.data?.length === 0 && (
            <Text as="p">{t('competencyMap.noSubjects')}</Text>
          )}
          {subjects.data?.length ? (
            query.isPending ? (
              <Text as="p" role="status">
                {t('competencyMap.loading')}
              </Text>
            ) : query.isError ? (
              <div>
                <Text as="p" role="alert">
                  {t('competencyMap.loadError')}
                </Text>
                <Button variant="soft" onClick={() => void query.refetch()}>
                  {t('trainer.retry')}
                </Button>
              </div>
            ) : query.data.importedAt === null ? (
              <Text as="p" color="gray">
                {t('competencyMap.empty')}
              </Text>
            ) : (
              <MapSummary summary={query.data} />
            )
          ) : null}
        </section>
        <section className={styles.card} aria-labelledby="create-subject-title">
          <Heading as="h2" size="4" id="create-subject-title">
            {t('competencyMap.createSubject')}
          </Heading>
          <Flex gap="2" align="end" wrap="wrap">
            <label>
              {t('competencyMap.subjectName')}
              <TextField.Root
                value={subjectName}
                onChange={(event) => setSubjectName(event.target.value)}
              />
            </label>
            <Button
              disabled={!subjectName.trim() || create.isPending}
              onClick={() => {
                const name = subjectName.trim()
                if (name)
                  create.mutate(name, {
                    onSuccess: (subject) => {
                      setSubjectName('')
                      setSubjectId(subject.id)
                    },
                  })
              }}
            >
              {t('competencyMap.createSubjectButton')}
            </Button>
          </Flex>
          {create.isError && (
            <Text role="alert" color="red">
              {t('competencyMap.createSubjectError')}
            </Text>
          )}
        </section>
        <section className={styles.card} aria-labelledby="upload-map-title">
          <div className={styles.sectionHeading}>
            <FileSpreadsheet aria-hidden="true" />
            <Heading as="h2" size="4" id="upload-map-title">
              {t('competencyMap.uploadTitle')}
            </Heading>
          </div>
          <Text as="p" color="gray">
            {t('competencyMap.formatHelp')}
          </Text>
          <Callout.Root color="amber">
            <Callout.Text>{t('competencyMap.replacementHelp')}</Callout.Text>
          </Callout.Root>
          <div className={styles.filePicker}>
            <Text as="label" htmlFor="map-file" weight="bold">
              {t('competencyMap.file')}
            </Text>
            <input
              id="map-file"
              type="file"
              accept=".csv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
              disabled={!selectedSubject || upload.isPending}
              onChange={(event) => selectFile(event.target.files?.[0])}
            />
            {file && (
              <Text as="p" color="gray" size="2">
                {file.name} ·{' '}
                {t('competencyMap.fileSize', {
                  size: (file.size / (1024 * 1024)).toLocaleString(
                    i18n.language,
                    { maximumFractionDigits: 2 },
                  ),
                })}
              </Text>
            )}
          </div>
          {fileError && (
            <Text as="p" role="alert" color="red">
              {fileError}
            </Text>
          )}
          {upload.isPending && (
            <Text as="p" role="status">
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
          <AlertDialog.Root>
            <AlertDialog.Trigger>
              <Button
                size="3"
                disabled={!selectedSubject || !file || upload.isPending}
              >
                <Upload size={18} aria-hidden="true" />
                {t('competencyMap.upload')}
              </Button>
            </AlertDialog.Trigger>
            <AlertDialog.Content maxWidth="480px">
              <AlertDialog.Title>
                {t('competencyMap.confirmTitle')}
              </AlertDialog.Title>
              <AlertDialog.Description>
                {t('competencyMap.confirmHelp', { filename: file?.name })}
              </AlertDialog.Description>
              <Flex gap="3" mt="5" justify="end" wrap="wrap">
                <AlertDialog.Cancel>
                  <Button variant="soft" color="gray">
                    {t('competencyMap.cancel')}
                  </Button>
                </AlertDialog.Cancel>
                <AlertDialog.Action>
                  <Button
                    onClick={() => {
                      if (file && selectedSubject && !upload.isPending)
                        upload.mutate({ file, subjectId })
                    }}
                  >
                    {t('competencyMap.confirm')}
                  </Button>
                </AlertDialog.Action>
              </Flex>
            </AlertDialog.Content>
          </AlertDialog.Root>
        </section>
        {upload.data && upload.variables?.subjectId === subjectId && (
          <section
            className={styles.card}
            aria-labelledby="import-result-title"
          >
            <div className={styles.sectionHeading}>
              <CheckCircle2 aria-hidden="true" />
              <Heading as="h2" size="4" id="import-result-title">
                {t('competencyMap.resultTitle')}
              </Heading>
            </div>
            <Text as="p" role="status" color="green">
              {t('competencyMap.success')}
            </Text>
            <MapSummary summary={upload.data} />
            {upload.data.unparsedTaskCellCount > 0 && (
              <Text as="p" color="amber">
                {t('competencyMap.unparsed', {
                  count: upload.data.unparsedTaskCellCount,
                })}
              </Text>
            )}
            {upload.data.warnings.length > 0 && (
              <>
                <Heading as="h3" size="3">
                  {t('competencyMap.warnings')}
                </Heading>
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
                              defaultValue: t('competencyMap.warning.unknown', {
                                code: warning.code,
                              }),
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

function MapSummary({ summary }: { summary: CompetencyMapSummary }) {
  const { t, i18n } = useTranslation()
  return (
    <>
      <Text as="p" color="gray" size="2">
        {t('competencyMap.revision', { revision: summary.revision })} ·{' '}
        {summary.importedAt !== null &&
          new Date(summary.importedAt * 1000).toLocaleString(i18n.language)}
      </Text>
      <dl className={styles.stats}>
        {(
          [
            'competencyCount',
            'constituentCount',
            'outcomeCount',
            'taskCount',
          ] as const
        ).map((key) => (
          <div key={key}>
            <dt>{t(`competencyMap.stats.${key}`)}</dt>
            <dd>{summary[key]}</dd>
          </div>
        ))}
      </dl>
    </>
  )
}
