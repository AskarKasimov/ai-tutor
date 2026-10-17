import {
  AlertDialog,
  Badge,
  Button,
  Dialog,
  Flex,
  Progress,
  Heading,
  Select,
  Text,
  TextField,
} from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import {
  BookOpen,
  Check,
  CheckCircle2,
  Clock,
  Download,
  Eye,
  EyeOff,
  FileText,
  ShieldCheck,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  downloadCourseMaterialRaw,
  type CourseMaterial,
} from '@/entities/competency-map'
import { CourseGraphApiError } from '@/entities/course-graph'
import { AccountMenu, useAuth } from '@/features/auth'
import {
  useDeleteCourseMaterialMutation,
  useSubjectMaterialsQuery,
  useUpdateCourseMaterialMetadataMutation,
  useUploadCourseMaterialsMutation,
  useReplaceCourseMaterialMutation,
  MaterialUploadQueue,
} from '@/features/import-competency-map'
import {
  useCourseGraphQuery,
  useLatestCourseGraphBuildQuery,
  useStartCourseGraphBuildMutation,
} from '@/features/course-graph-build'
import type { CourseMaterialKind } from '@/entities/competency-map'
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
        <Heading as="h1">{t('courseGraph.title')}</Heading>
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
  const [subjectName, setSubjectName] = useState('')
  // Keep the newly created course visible while the catalogue query refreshes.
  const [createdId, setCreatedId] = useState('')
  const [createCourseOpen, setCreateCourseOpen] = useState(false)
  const [selectedSubjectId, setSelectedSubjectId] = useState('')
  const subject =
    subjects.data?.find((candidate) => candidate.id === selectedSubjectId) ??
    subjects.data?.[0]
  const subjectId = selectedSubjectId || subject?.id || createdId
  const subjectDisplayName =
    subjects.data?.find((candidate) => candidate.id === subjectId)?.name ??
    (createdId === subjectId ? subjectName.trim() : subject?.name)

  // Course materials state and mutations
  const materialsQuery = useSubjectMaterialsQuery(userId, subjectId)
  const uploadMaterials = useUploadCourseMaterialsMutation(userId, subjectId)
  const replaceMaterial = useReplaceCourseMaterialMutation(userId, subjectId)
  const deleteMaterial = useDeleteCourseMaterialMutation(userId, subjectId)
  const updateMetadata = useUpdateCourseMaterialMetadataMutation(
    userId,
    subjectId,
  )
  const latestBuild = useLatestCourseGraphBuildQuery(userId, subjectId)
  const courseGraph = useCourseGraphQuery(userId, subjectId)
  const startBuild = useStartCourseGraphBuildMutation(userId, subjectId)
  const [previewId, setPreviewId] = useState<string | null>(null)

  const busy =
    create.isPending ||
    uploadMaterials.isPending ||
    replaceMaterial.isPending ||
    deleteMaterial.isPending ||
    updateMetadata.isPending
  const materials = materialsQuery.data ?? []
  const graphStateResolved =
    materialsQuery.isSuccess &&
    latestBuild.isSuccess &&
    (courseGraph.isSuccess ||
      (courseGraph.error instanceof CourseGraphApiError &&
        courseGraph.error.status === 404))
  const parsedCount = materials.filter(
    (material) => material.status === 'parsed',
  ).length
  const failedCount = materials.filter(
    (material) => material.status === 'failed',
  ).length
  const missingMetadataCount = materials.filter(
    (material) => !material.week || !material.kind,
  ).length
  const graphBlockers = [
    ...(materials.length === 0 ? [t('courseGraph.blockNoMaterials')] : []),
    ...(parsedCount === 0 && materials.length > 0
      ? [t('courseGraph.blockNoParsed')]
      : []),
    ...(missingMetadataCount > 0
      ? [t('courseGraph.blockMetadata', { count: missingMetadataCount })]
      : []),
    ...(failedCount > 0
      ? [t('courseGraph.blockFailed', { count: failedCount })]
      : []),
    ...(materials.some((material) => material.status === 'pending')
      ? [t('courseGraph.blockPending')]
      : []),
  ]
  const hasGraph = courseGraph.data !== undefined

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <div className={styles.brand}>
          <ShieldCheck size={24} aria-hidden="true" />
          <Text weight="bold">{t('competencyMap.admin')}</Text>
        </div>
        <AccountMenu canLogout={false} />
      </header>
      <main className={styles.content}>
        <div className={styles.intro}>
          <Heading as="h1" size="8">
            {t('courseGraph.title')}
          </Heading>
          <Text as="p" color="gray">
            {t(subjectId ? 'courseGraph.subtitle' : 'competencyMap.subtitle')}
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
            {create.isError && (
              <Text as="p" role="alert" color="red" size="2">
                {t('competencyMap.createSubjectError')}
              </Text>
            )}
            <Button
              size="3"
              color="green"
              className={styles.submit}
              disabled={!subjectName.trim() || busy}
              onClick={() => {
                const name = subjectName.trim()
                if (!name) return
                create.mutate(name, {
                  onSuccess: (created) => setCreatedId(created.id),
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
                  {subjectDisplayName}
                </Heading>
                {(subjects.data?.length ?? 0) > 1 && (
                  <Select.Root
                    value={subjectId}
                    onValueChange={setSelectedSubjectId}
                  >
                    <Select.Trigger
                      aria-label={t('courseGraph.selectCourse')}
                    />
                    <Select.Content>
                      {subjects.data?.map((candidate) => (
                        <Select.Item key={candidate.id} value={candidate.id}>
                          {candidate.name}
                        </Select.Item>
                      ))}
                    </Select.Content>
                  </Select.Root>
                )}
                <Dialog.Root
                  open={createCourseOpen}
                  onOpenChange={setCreateCourseOpen}
                >
                  <Dialog.Trigger>
                    <Button
                      variant="soft"
                      size="2"
                      onClick={() => setCreateCourseOpen(true)}
                    >
                      {t('courseGraph.createAnotherCourse')}
                    </Button>
                  </Dialog.Trigger>
                  <Dialog.Content maxWidth="420px">
                    <Dialog.Title>
                      {t('courseGraph.createCourseTitle')}
                    </Dialog.Title>
                    <TextField.Root
                      aria-label={t('competencyMap.subjectName')}
                      placeholder={t('competencyMap.subjectPlaceholder')}
                      value={subjectName}
                      onChange={(event) => setSubjectName(event.target.value)}
                    />
                    {create.isError && (
                      <Text as="p" role="alert" color="red" size="2">
                        {t('competencyMap.createSubjectError')}
                      </Text>
                    )}
                    <Flex gap="3" mt="4" justify="end">
                      <Dialog.Close>
                        <Button variant="soft" color="gray">
                          {t('competencyMap.cancel')}
                        </Button>
                      </Dialog.Close>
                      <Button
                        disabled={!subjectName.trim() || create.isPending}
                        onClick={() => {
                          const name = subjectName.trim()
                          if (!name) return
                          create.mutate(name, {
                            onSuccess: (created) => {
                              setCreatedId(created.id)
                              setSelectedSubjectId(created.id)
                              setCreateCourseOpen(false)
                            },
                          })
                        }}
                      >
                        {t('courseGraph.createCourseAction')}
                      </Button>
                    </Flex>
                  </Dialog.Content>
                </Dialog.Root>
              </div>
            </section>

            <nav
              className={styles.pipeline}
              aria-label={t('courseGraph.pipeline')}
            >
              <div data-active="true">
                <span>1</span>
                <Text weight="bold">{t('courseGraph.materialsStep')}</Text>
              </div>
              <div
                data-ready={
                  (parsedCount > 0 && graphBlockers.length === 0) ||
                  hasGraph ||
                  undefined
                }
              >
                <span>2</span>
                <Text weight="bold">{t('courseGraph.graphStep')}</Text>
              </div>
              <div data-ready={hasGraph || undefined}>
                <span>3</span>
                <Text weight="bold">{t('courseGraph.readyStep')}</Text>
              </div>
            </nav>

            <section
              className={styles.card}
              aria-labelledby="materials-drop-title"
            >
              <div className={styles.sectionHeading}>
                <BookOpen aria-hidden="true" />
                <Heading as="h2" size="4" id="materials-drop-title">
                  {t('competencyMap.materialsTitle')}
                </Heading>
              </div>

              <MaterialUploadQueue
                userId={userId}
                subjectId={subjectId}
                disabled={busy}
              />
            </section>

            <section
              className={styles.card}
              aria-labelledby="materials-list-title"
            >
              <div className={styles.sectionHeading}>
                <FileText aria-hidden="true" />
                <Heading as="h2" size="4" id="materials-list-title">
                  {t('competencyMap.materialsListTitle')}
                </Heading>
              </div>

              {replaceMaterial.isError && (
                <Text as="p" role="alert" color="red">
                  {replaceMaterial.error instanceof Error
                    ? replaceMaterial.error.message
                    : t('competencyMap.requestError')}
                </Text>
              )}
              {materialsQuery.isPending ? (
                <Text as="p" role="status" color="gray" size="2">
                  {t('competencyMap.loading')}
                </Text>
              ) : materialsQuery.isError ? (
                <Flex gap="3" align="center" wrap="wrap">
                  <Text as="p" role="alert" color="red" size="2">
                    {t('competencyMap.loadError')}
                  </Text>
                  <Button
                    variant="ghost"
                    size="1"
                    onClick={() => void materialsQuery.refetch()}
                  >
                    {t('trainer.retry')}
                  </Button>
                </Flex>
              ) : materialsQuery.data && materialsQuery.data.length > 0 ? (
                <div className={styles.materialsList}>
                  {materialsQuery.data.map((mat) => (
                    <MaterialItemCard
                      key={mat.id}
                      material={mat}
                      subjectId={subjectId}
                      previewOpen={previewId === mat.id}
                      onTogglePreview={() =>
                        setPreviewId((current) =>
                          current === mat.id ? null : mat.id,
                        )
                      }
                      onDelete={() =>
                        deleteMaterial.mutate({
                          materialId: mat.id,
                          subjectId,
                        })
                      }
                      onMetadataChange={(patch) =>
                        updateMetadata.mutate({ materialId: mat.id, patch })
                      }
                      onReplace={(file) =>
                        replaceMaterial.mutate({ materialId: mat.id, file })
                      }
                      busy={busy}
                    />
                  ))}
                </div>
              ) : (
                <Text as="p" color="gray" size="2">
                  {t('competencyMap.materialsEmpty')}
                </Text>
              )}
            </section>

            <section className={styles.card} aria-labelledby="graph-title">
              <div className={styles.sectionHeading}>
                <Sparkles aria-hidden="true" />
                <Heading as="h2" size="4" id="graph-title">
                  {t('courseGraph.graphTitle')}
                </Heading>
                <Badge
                  color={
                    hasGraph ? 'green' : graphBlockers.length ? 'amber' : 'blue'
                  }
                >
                  {hasGraph
                    ? t('courseGraph.graphStored')
                    : t('courseGraph.graphNotReady')}
                </Badge>
              </div>
              <Text as="p" color="gray" size="2">
                {t('courseGraph.materialSummary', {
                  total: materials.length,
                  parsed: parsedCount,
                  failed: failedCount,
                })}
              </Text>
              {graphBlockers.length > 0 && !hasGraph && (
                <div className={styles.warning} role="status">
                  <Text weight="bold">{t('courseGraph.blockedTitle')}</Text>
                  <ul>
                    {graphBlockers.map((reason) => (
                      <li key={reason}>{reason}</li>
                    ))}
                  </ul>
                </div>
              )}
              {latestBuild.isError && (
                <div>
                  <Text as="p" role="alert" color="red">
                    {t('courseGraph.statusError')}
                  </Text>
                  <Button
                    variant="ghost"
                    onClick={() => void latestBuild.refetch()}
                  >
                    {t('trainer.retry')}
                  </Button>
                </div>
              )}
              {(latestBuild.data?.status === 'queued' ||
                latestBuild.data?.status === 'running') && (
                <div role="status" className={styles.buildProgress}>
                  <Text weight="bold">
                    {t(`courseGraph.status.${latestBuild.data.status}`)}
                  </Text>
                  <Text size="2" color="gray">
                    {latestBuild.data.stage
                      ? t(`courseGraph.stages.${latestBuild.data.stage}`)
                      : t('courseGraph.stagePreparing')}
                  </Text>
                  <Progress
                    value={
                      latestBuild.data.stage
                        ? ([
                            'cleaning',
                            'units',
                            'dedup',
                            'prerequisites',
                            'validation',
                          ].indexOf(latestBuild.data.stage) +
                            1) *
                          20
                        : 8
                    }
                  />
                </div>
              )}
              {latestBuild.data?.status === 'failed' && (
                <div className={styles.error} role="alert">
                  <Text weight="bold">{t('courseGraph.buildFailed')}</Text>
                  <Text as="p">
                    {latestBuild.data.error?.message ??
                      t('courseGraph.buildFailedHint')}
                  </Text>
                </div>
              )}
              {courseGraph.isError &&
                courseGraph.error instanceof CourseGraphApiError &&
                courseGraph.error.status !== 404 && (
                  <div>
                    <Text as="p" role="alert" color="red">
                      {t('courseGraph.graphReadError')}
                    </Text>
                    <Button
                      variant="ghost"
                      onClick={() => void courseGraph.refetch()}
                    >
                      {t('trainer.retry')}
                    </Button>
                  </div>
                )}
              {hasGraph && (
                <div className={styles.success} role="status">
                  <CheckCircle2 aria-hidden="true" />
                  <span>
                    {t('courseGraph.readyMessage', {
                      count: courseGraph.data.nodes.length,
                    })}
                  </span>
                </div>
              )}
              {startBuild.isError && (
                <Text as="p" role="alert" color="red">
                  {startBuild.error instanceof Error
                    ? startBuild.error.message
                    : t('courseGraph.buildFailedHint')}
                </Text>
              )}
              <AlertDialog.Root>
                <AlertDialog.Trigger>
                  <Button
                    size="3"
                    color="green"
                    disabled={
                      !graphStateResolved ||
                      graphBlockers.length > 0 ||
                      latestBuild.data?.status === 'queued' ||
                      latestBuild.data?.status === 'running' ||
                      startBuild.isPending
                    }
                  >
                    <Sparkles size={17} aria-hidden="true" />
                    {hasGraph
                      ? t('courseGraph.rebuild')
                      : t('courseGraph.build')}
                  </Button>
                </AlertDialog.Trigger>
                <AlertDialog.Content maxWidth="440px">
                  <AlertDialog.Title>
                    {hasGraph
                      ? t('courseGraph.rebuildTitle')
                      : t('courseGraph.buildTitle')}
                  </AlertDialog.Title>
                  <AlertDialog.Description>
                    {hasGraph
                      ? t('courseGraph.rebuildWarning')
                      : t('courseGraph.buildConfirm')}
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
                        onClick={() => startBuild.mutate(subjectId)}
                      >
                        {t('courseGraph.confirmBuild')}
                      </Button>
                    </AlertDialog.Action>
                  </Flex>
                </AlertDialog.Content>
              </AlertDialog.Root>
            </section>
          </>
        )}
      </main>
    </div>
  )
}

function MaterialItemCard({
  material,
  subjectId,
  previewOpen,
  onTogglePreview,
  onDelete,
  onMetadataChange,
  onReplace,
  busy,
}: {
  material: CourseMaterial
  subjectId: string
  previewOpen: boolean
  onTogglePreview: () => void
  onDelete: () => void
  onMetadataChange: (patch: {
    week?: number
    kind?: CourseMaterialKind
  }) => void
  onReplace: (file: File) => void
  busy: boolean
}) {
  const { t, i18n } = useTranslation()
  const [downloading, setDownloading] = useState(false)
  const [downloadError, setDownloadError] = useState<string | null>(null)
  const [weekValue, setWeekValue] = useState(String(material.week ?? ''))
  useEffect(() => setWeekValue(String(material.week ?? '')), [material.week])
  const formattedSize =
    material.fileSize > 1024 * 1024
      ? `${(material.fileSize / (1024 * 1024)).toFixed(1)} МБ`
      : `${Math.round(material.fileSize / 1024)} КБ`

  const dateStr = new Date(material.createdAt * 1000).toLocaleString(
    i18n.language,
    { dateStyle: 'medium', timeStyle: 'short' },
  )

  const statusTone =
    material.status === 'parsed'
      ? 'green'
      : material.status === 'failed'
        ? 'red'
        : 'amber'

  async function handleDownload() {
    setDownloading(true)
    setDownloadError(null)
    try {
      await downloadCourseMaterialRaw(subjectId, material.id, material.filename)
    } catch (err) {
      setDownloadError(
        err instanceof Error
          ? err.message
          : t('competencyMap.materialsDownloadError'),
      )
    } finally {
      setDownloading(false)
    }
  }

  return (
    <div className={styles.materialCard}>
      <div className={styles.materialHeader}>
        <Flex gap="2" align="center">
          <FileText size={18} color="var(--accent-9)" />
          <Text weight="bold" size="3">
            {material.filename}
          </Text>
          <Badge color={statusTone} size="1">
            {t(`competencyMap.materialsStatus.${material.status}`)}
          </Badge>
        </Flex>

        <Flex gap="2" align="center">
          <Button
            size="1"
            variant="soft"
            color="gray"
            onClick={onTogglePreview}
          >
            {previewOpen ? <EyeOff size={14} /> : <Eye size={14} />}
            {t('competencyMap.materialsPreview')}
          </Button>

          <Button
            size="1"
            variant="ghost"
            color="gray"
            disabled={downloading}
            onClick={() => void handleDownload()}
            aria-label={`Скачать ${material.filename}`}
          >
            <Download size={14} />
          </Button>

          <AlertDialog.Root>
            <AlertDialog.Trigger>
              <Button size="1" color="red" variant="soft" disabled={busy}>
                <Trash2 size={14} />
                {t('competencyMap.materialsDelete')}
              </Button>
            </AlertDialog.Trigger>
            <AlertDialog.Content maxWidth="400px">
              <AlertDialog.Title>
                {t('competencyMap.materialsDelete')}
              </AlertDialog.Title>
              <AlertDialog.Description>
                {t('competencyMap.materialsDeleteConfirm')}
              </AlertDialog.Description>
              <Flex gap="3" mt="4" justify="end">
                <AlertDialog.Cancel>
                  <Button variant="soft" color="gray">
                    {t('competencyMap.cancel')}
                  </Button>
                </AlertDialog.Cancel>
                <AlertDialog.Action>
                  <Button color="red" onClick={onDelete}>
                    {t('competencyMap.confirm')}
                  </Button>
                </AlertDialog.Action>
              </Flex>
            </AlertDialog.Content>
          </AlertDialog.Root>
        </Flex>
      </div>

      <div className={styles.materialMeta}>
        <span>{t('competencyMap.materialsSize', { size: formattedSize })}</span>
        <span>•</span>
        <span>
          {t('competencyMap.materialsCharCount', {
            count: material.charCount.toLocaleString(i18n.language),
          })}
        </span>
        <span>•</span>
        <Flex gap="1" align="center">
          <Clock size={12} />
          <span>{dateStr}</span>
        </Flex>
      </div>

      <label className={styles.replaceMaterial}>
        <span>{t('competencyMap.materialReplace')}</span>
        <input
          type="file"
          accept=".pdf,application/pdf"
          disabled={busy}
          onChange={(event) => {
            const file = event.currentTarget.files?.[0]
            if (file) onReplace(file)
            event.currentTarget.value = ''
          }}
        />
      </label>
      <div className={styles.materialClassification}>
        <label>
          {t('courseGraph.weekLabel')}
          <TextField.Root
            type="number"
            min="1"
            step="1"
            value={weekValue}
            disabled={busy}
            aria-label={`${t('courseGraph.weekLabel')}: ${material.filename}`}
            onChange={(event) => setWeekValue(event.target.value)}
            onBlur={() => {
              const value = weekValue
              if (
                /^\d+$/.test(value) &&
                Number(value) > 0 &&
                Number(value) !== material.week
              )
                onMetadataChange({ week: Number(value) })
            }}
          />
        </label>
        <label>
          {t('courseGraph.kindLabel')}
          <Select.Root
            value={material.kind ?? ''}
            onValueChange={(value) =>
              onMetadataChange({ kind: value as CourseMaterialKind })
            }
          >
            <Select.Trigger
              aria-label={`${t('courseGraph.kindLabel')}: ${material.filename}`}
              placeholder={t('courseGraph.kindMissing')}
              disabled={busy}
            />
            <Select.Content>
              <Select.Item value="lecture">
                {t('courseGraph.kinds.lecture')}
              </Select.Item>
              <Select.Item value="lab">
                {t('courseGraph.kinds.lab')}
              </Select.Item>
              <Select.Item value="tutorial">
                {t('courseGraph.kinds.tutorial')}
              </Select.Item>
            </Select.Content>
          </Select.Root>
        </label>
        {(!material.week || !material.kind) && (
          <Badge color="amber">{t('courseGraph.metadataMissing')}</Badge>
        )}
        {material.status === 'failed' && material.errorMessage && (
          <Text color="red" size="2">
            {material.errorMessage}
          </Text>
        )}
      </div>

      {downloadError && (
        <Text as="p" role="alert" color="red" size="2">
          {downloadError}
        </Text>
      )}

      {previewOpen && (
        <div className={styles.materialPreview}>
          {material.parsedText || '(текст отсутствует)'}
        </div>
      )}
    </div>
  )
}
