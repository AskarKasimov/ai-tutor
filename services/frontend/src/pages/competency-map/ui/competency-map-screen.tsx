import {
  AlertDialog,
  Badge,
  Button,
  Callout,
  Flex,
  Heading,
  Progress,
  SegmentedControl,
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
  Layers,
  Network,
  ShieldCheck,
  Sparkles,
  Target,
  Trash2,
  TriangleAlert,
  Upload,
  X,
} from 'lucide-react'
import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  downloadCourseMaterialRaw,
  type CourseGraphSnapshot,
  type CourseMaterial,
} from '@/entities/competency-map'
import { AccountMenu, useAuth } from '@/features/auth'
import {
  materialInProgress,
  useBuildCourseGraphMutation,
  useCourseGraphQuery,
  useDeleteCourseMaterialMutation,
  useSubjectMaterialsQuery,
  useUploadCourseMaterialsMutation,
} from '@/features/import-competency-map'
import {
  useCreateSubjectMutation,
  useSubjectsQuery,
} from '@/features/subject-selection'
import styles from '@/pages/competency-map/ui/competency-map.module.scss'

// React Flow and the layout engine load only when the graph is opened.
const CourseGraphCanvas = lazy(() => import('./course-graph-canvas'))

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

  // Course materials state and mutations
  const materialsQuery = useSubjectMaterialsQuery(userId, subjectId)
  const materialsBusy = materialsQuery.data?.some(materialInProgress) ?? false
  const courseGraphQuery = useCourseGraphQuery(userId, subjectId, materialsBusy)
  const buildGraph = useBuildCourseGraphMutation(userId, subjectId)
  const refetchGraph = courseGraphQuery.refetch
  // Once the last material finishes, read inputs_ready without waiting for focus.
  useEffect(() => {
    if (!materialsBusy && subjectId) void refetchGraph()
  }, [materialsBusy, subjectId, refetchGraph])
  const uploadMaterials = useUploadCourseMaterialsMutation(userId, subjectId)
  const deleteMaterial = useDeleteCourseMaterialMutation(userId, subjectId)
  const [lectureFiles, setLectureFiles] = useState<File[]>([])
  const [materialsFileError, setMaterialsFileError] = useState<string | null>(
    null,
  )
  const [previewId, setPreviewId] = useState<string | null>(null)
  const [uploadWarning, setUploadWarning] = useState<string | null>(null)

  function selectLectureFiles(selected: FileList | null) {
    uploadMaterials.reset()
    setMaterialsFileError(null)
    setUploadWarning(null)
    if (!selected || selected.length === 0) return
    const validFiles: File[] = []
    for (let i = 0; i < selected.length; i++) {
      const f = selected[i]
      if (!/\.(pdf|md|txt|markdown)$/i.test(f.name)) {
        setMaterialsFileError(t('competencyMap.materialsInvalidType'))
        return
      }
      if (f.size > 50 * 1024 * 1024) {
        setMaterialsFileError(t('competencyMap.materialsTooLarge'))
        return
      }
      if (f.size === 0) {
        setMaterialsFileError(t('competencyMap.emptyFile'))
        return
      }
      validFiles.push(f)
    }
    // Skip names already chosen, in earlier picks and within this one.
    const seenNames = new Set(lectureFiles.map((f) => f.name.toLowerCase()))
    const unique = validFiles.filter((f) => {
      const name = f.name.toLowerCase()
      if (seenNames.has(name)) return false
      seenNames.add(name)
      return true
    })
    if (lectureFiles.length + unique.length > 48) {
      setMaterialsFileError(t('competencyMap.materialsTooManyFiles'))
      return
    }
    setLectureFiles((prev) => [...prev, ...unique])
  }

  function removeLectureFile(index: number) {
    setLectureFiles((prev) => prev.filter((_, i) => i !== index))
  }

  const [subjectName, setSubjectName] = useState('')
  const busy =
    create.isPending || uploadMaterials.isPending || deleteMaterial.isPending

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
                  {subject?.name ?? subjectName.trim()}
                </Heading>
              </div>
            </section>

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

              <LectureFilesZone
                files={lectureFiles}
                disabled={busy}
                onSelect={selectLectureFiles}
                onRemove={removeLectureFile}
              />

              {materialsFileError && (
                <Text as="p" role="alert" color="red" size="2">
                  {materialsFileError}
                </Text>
              )}

              {uploadMaterials.isPending && (
                <MaterialsUploadProgressBar filesCount={lectureFiles.length} />
              )}

              {uploadMaterials.isError && (
                <div role="alert" className={styles.error}>
                  <Text as="p">
                    {uploadMaterials.error instanceof Error
                      ? uploadMaterials.error.message
                      : t('competencyMap.unknownResult')}
                  </Text>
                </div>
              )}

              {uploadMaterials.isSuccess && (
                <Text as="p" role="status" className={styles.success}>
                  <CheckCircle2 size={20} aria-hidden="true" />
                  {t('competencyMap.materialsSuccess')}
                </Text>
              )}

              {uploadWarning && (
                <div role="alert" className={styles.warning}>
                  <Text as="p">{uploadWarning}</Text>
                </div>
              )}

              {lectureFiles.length > 0 && (
                <div className={styles.actions}>
                  <Button
                    size="3"
                    color="green"
                    className={styles.submit}
                    disabled={lectureFiles.length === 0 || busy}
                    onClick={() => {
                      setUploadWarning(null)
                      uploadMaterials.mutate(
                        { files: lectureFiles, subjectId },
                        {
                          onSuccess: (result) => {
                            setLectureFiles([])
                            if (result.failed && result.failed.length > 0) {
                              const failedMsg = result.failed
                                .map((f) => `${f.filename}: ${f.message}`)
                                .join('; ')
                              setUploadWarning(
                                t('competencyMap.materialsPartialWarning', {
                                  failed: failedMsg,
                                }),
                              )
                            }
                          },
                        },
                      )
                    }}
                  >
                    <Check size={18} aria-hidden="true" />
                    {t('competencyMap.materialsSubmit')} ({lectureFiles.length})
                  </Button>
                  <Button
                    size="3"
                    color="red"
                    variant="soft"
                    disabled={busy}
                    onClick={() => setLectureFiles([])}
                  >
                    <X size={18} aria-hidden="true" />
                    {t('competencyMap.cancelEdit')}
                  </Button>
                </div>
              )}
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

            <CourseGraphCard
              materials={materialsQuery.data}
              snapshot={courseGraphQuery.data}
              loading={courseGraphQuery.isPending}
              error={courseGraphQuery.isError}
              onRetry={() => void courseGraphQuery.refetch()}
              onBuild={() => buildGraph.mutate()}
              buildPending={buildGraph.isPending}
              buildError={buildGraph.error?.message}
            />
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
  busy,
}: {
  material: CourseMaterial
  subjectId: string
  previewOpen: boolean
  onTogglePreview: () => void
  onDelete: () => void
  busy: boolean
}) {
  const { t, i18n } = useTranslation()
  const [downloading, setDownloading] = useState(false)
  const [downloadError, setDownloadError] = useState<string | null>(null)
  const formattedSize =
    material.fileSize > 1024 * 1024
      ? `${(material.fileSize / (1024 * 1024)).toFixed(1)} МБ`
      : `${Math.round(material.fileSize / 1024)} КБ`

  const dateStr = new Date(material.createdAt * 1000).toLocaleString(
    i18n.language,
    { dateStyle: 'medium', timeStyle: 'short' },
  )

  const statusTone =
    material.processing?.status === 'failed' ||
    material.cleaning?.status === 'failed'
      ? 'red'
      : material.status === 'parsed'
        ? 'green'
        : material.status === 'failed'
          ? 'red'
          : 'amber'
  const cleaningActive =
    material.cleaning?.status === 'pending' ||
    material.cleaning?.status === 'processing'
  const processingStage = cleaningActive
    ? 'cleaning'
    : material.processing?.stage
  const lastSignalAt = cleaningActive
    ? material.cleaning?.updatedAt
    : material.processing?.updatedAt
  const errorCoveredByReport =
    material.errorMessage &&
    material.ocrWarnings?.some((warning) =>
      material.errorMessage?.includes(warning),
    )
  const showErrorMessage = material.errorMessage && !errorCoveredByReport

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

      {processingStage && (
        <Flex gap="2" align="center" wrap="wrap" role="status">
          <Badge
            color={
              material.processing?.status === 'failed' ||
              material.processing?.status === 'ready'
                ? material.processing.status === 'failed'
                  ? 'red'
                  : 'green'
                : 'blue'
            }
            size="1"
          >
            {t(`competencyMap.materialStage.${processingStage}`)}
          </Badge>
          {lastSignalAt !== undefined && lastSignalAt > 0 && (
            <Text size="1" color="gray">
              {t('competencyMap.materialLastSignal', {
                date: new Date(lastSignalAt * 1000).toLocaleString(
                  i18n.language,
                  { dateStyle: 'medium', timeStyle: 'short' },
                ),
              })}
            </Text>
          )}
        </Flex>
      )}

      {showErrorMessage && (
        <Callout.Root
          role={
            material.status === 'failed' ||
            material.processing?.status === 'failed' ||
            material.cleaning?.status === 'failed'
              ? 'alert'
              : 'status'
          }
          color={
            material.status === 'failed' ||
            material.processing?.status === 'failed' ||
            material.cleaning?.status === 'failed'
              ? 'red'
              : 'amber'
          }
          size="1"
        >
          <Callout.Icon>
            <TriangleAlert size={14} />
          </Callout.Icon>
          <Callout.Text>{material.errorMessage}</Callout.Text>
        </Callout.Root>
      )}

      {material.ocrWarnings && material.ocrWarnings.length > 0 && (
        <Callout.Root color="amber" size="1">
          <Callout.Icon>
            <TriangleAlert size={14} />
          </Callout.Icon>
          <Callout.Text>
            <Text size="1" weight="bold">
              {t('competencyMap.materialWarnings')}:
            </Text>
            <ul className={styles.materialWarnings}>
              {material.ocrWarnings.map((warning, index) => (
                <li key={index}>
                  <Text size="1">{warning}</Text>
                </li>
              ))}
            </ul>
          </Callout.Text>
        </Callout.Root>
      )}

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

function LectureFilesZone({
  files,
  disabled,
  onSelect,
  onRemove,
}: {
  files: File[]
  disabled: boolean
  onSelect: (files: FileList | null) => void
  onRemove: (index: number) => void
}) {
  const { t } = useTranslation()
  const [dragging, setDragging] = useState(false)
  return (
    <div>
      <label
        className={styles.dropzone}
        data-dragging={dragging || undefined}
        data-disabled={disabled || undefined}
        onDragOver={(event) => {
          event.preventDefault()
          if (!disabled) setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(event) => {
          event.preventDefault()
          setDragging(false)
          if (!disabled) onSelect(event.dataTransfer.files)
        }}
      >
        <input
          type="file"
          multiple
          className={styles.fileInput}
          aria-label={t('competencyMap.materialsDropTitle')}
          accept=".pdf,.md,.txt,.markdown"
          disabled={disabled}
          onChange={(event) => {
            onSelect(event.target.files)
            event.target.value = ''
          }}
        />
        <span className={styles.dropzoneIcon} aria-hidden="true">
          <Upload size={22} />
        </span>
        <span className={styles.dropzoneText}>
          <Text weight="bold">{t('competencyMap.materialsDropTitle')}</Text>
          <Text size="2" color="gray">
            {t('competencyMap.materialsDropHint')}
          </Text>
        </span>
      </label>

      {files.length > 0 && (
        <div className={styles.fileChips}>
          {files.map((f, idx) => (
            <span key={idx} className={styles.fileChip}>
              <FileText size={14} aria-hidden="true" />
              <span>
                {f.name} ({(f.size / 1024).toFixed(0)} КБ)
              </span>
              {!disabled && (
                <button
                  type="button"
                  aria-label={`Удалить ${f.name}`}
                  onClick={() => onRemove(idx)}
                >
                  <X size={12} />
                </button>
              )}
            </span>
          ))}
        </div>
      )}
    </div>
  )
}

function MaterialsUploadProgressBar({ filesCount }: { filesCount: number }) {
  const { t } = useTranslation()
  const [percent, setPercent] = useState(10)

  useEffect(() => {
    const timer = setInterval(() => {
      setPercent((prev) => {
        if (prev >= 95) return prev
        const remaining = 95 - prev
        const step = Math.max(1, Math.floor(remaining / 8))
        return Math.min(95, prev + step)
      })
    }, 600)
    return () => clearInterval(timer)
  }, [])

  return (
    <div className={styles.uploadProgressBox} role="status" aria-live="polite">
      <div className={styles.uploadProgressHeader}>
        <Flex gap="2" align="center">
          <Sparkles size={16} aria-hidden="true" className={styles.spinIcon} />
          <Text size="2" weight="medium">
            {t('competencyMap.materialsUploading')}
            {filesCount > 1 ? ` (${filesCount})` : ''}
          </Text>
        </Flex>
        <Text size="2" color="gray" weight="bold">
          {percent}%
        </Text>
      </div>
      <Progress value={percent} size="2" color="blue" />
    </div>
  )
}

function CourseGraphCard({
  materials,
  snapshot,
  loading,
  error,
  onRetry,
  onBuild,
  buildPending,
  buildError,
}: {
  materials?: CourseMaterial[]
  snapshot?: CourseGraphSnapshot
  loading: boolean
  error: boolean
  onRetry: () => void
  onBuild: () => void
  buildPending: boolean
  buildError?: string
}) {
  const { t } = useTranslation()
  const [treeOpen, setTreeOpen] = useState(false)
  const [mode, setMode] = useState<'canvas' | 'outline'>('canvas')
  const materialNames = useMemo(
    () => new Map((materials ?? []).map((m) => [m.id, m.filename])),
    [materials],
  )

  const status = snapshot?.status ?? 'not_requested'
  const isBuilding = status === 'processing' || status === 'pending'
  const graph = snapshot?.graph
  const topicsCount = graph?.topics?.length ?? 0
  const prereqsCount = graph?.prerequisites?.length ?? 0
  const hierarchyCount = graph?.hierarchy?.length ?? 0

  return (
    <section className={styles.card} aria-labelledby="course-graph-title">
      <div className={styles.sectionHeading}>
        <Network aria-hidden="true" />
        <Heading as="h2" size="4" id="course-graph-title">
          {t('competencyMap.graphTitle')}
        </Heading>
      </div>

      <Text size="2" color="gray">
        {t('competencyMap.graphSubtitle')}
      </Text>

      {loading ? (
        <Text as="p" role="status" color="gray" size="2">
          {t('competencyMap.loading')}
        </Text>
      ) : error ? (
        <Flex gap="3" align="center" wrap="wrap">
          <Text as="p" color="red" size="2">
            {t('competencyMap.loadError')}
          </Text>
          <Button variant="ghost" size="1" onClick={onRetry}>
            {t('trainer.retry')}
          </Button>
        </Flex>
      ) : (
        <div className={styles.graphBody}>
          <Flex align="center" gap="3" wrap="wrap">
            <span className={styles.graphBadge} data-status={status}>
              {isBuilding ? (
                <>
                  <Clock
                    size={14}
                    className={styles.spinIcon}
                    aria-hidden="true"
                  />
                  {t('competencyMap.graphBuilding')}
                </>
              ) : status === 'ready' ? (
                <>
                  <CheckCircle2 size={14} aria-hidden="true" />
                  {t('competencyMap.graphReady')}
                </>
              ) : status === 'failed' ? (
                <>
                  <TriangleAlert size={14} aria-hidden="true" />
                  {t('competencyMap.graphFailed')}
                </>
              ) : (
                t('competencyMap.graphNotRequested')
              )}
            </span>

            {snapshot?.stale && topicsCount > 0 && (
              <Badge color="amber" size="1">
                {t('competencyMap.graphStale')}
              </Badge>
            )}
          </Flex>

          <Flex align="center" gap="3" wrap="wrap">
            <Button
              size="2"
              color="green"
              disabled={!snapshot?.inputsReady || isBuilding || buildPending}
              onClick={onBuild}
            >
              <Network size={16} aria-hidden="true" />
              {t('competencyMap.graphBuild')}
            </Button>
            {!snapshot?.inputsReady && !isBuilding && (
              <Text size="2" color="gray">
                {t('competencyMap.graphInputsNotReady')}
              </Text>
            )}
          </Flex>
          {buildError && (
            <Text as="p" role="alert" color="red" size="2">
              {buildError}
            </Text>
          )}

          {isBuilding && (
            <Text size="2" color="gray">
              {t('competencyMap.graphBuildingHint')}
            </Text>
          )}

          {status === 'failed' && snapshot?.errorMessage && (
            <Callout.Root color="red" size="1">
              <Callout.Icon>
                <TriangleAlert size={14} />
              </Callout.Icon>
              <Callout.Text>{snapshot.errorMessage}</Callout.Text>
            </Callout.Root>
          )}

          {graph && (status === 'ready' || snapshot?.stale) && (
            <>
              <dl className={styles.stats}>
                <div className={styles.statRow} data-tone="blue">
                  <span className={styles.statIcon} aria-hidden="true">
                    <BookOpen size={16} />
                  </span>
                  <dt>{t('competencyMap.graphStats.topics')}</dt>
                  <dd>{topicsCount}</dd>
                </div>
                <div className={styles.statRow} data-tone="green">
                  <span className={styles.statIcon} aria-hidden="true">
                    <Target size={16} />
                  </span>
                  <dt>{t('competencyMap.graphStats.prerequisites')}</dt>
                  <dd>{prereqsCount}</dd>
                </div>
                <div className={styles.statRow} data-tone="violet">
                  <span className={styles.statIcon} aria-hidden="true">
                    <Layers size={16} />
                  </span>
                  <dt>{t('competencyMap.graphStats.hierarchy')}</dt>
                  <dd>{hierarchyCount}</dd>
                </div>
              </dl>

              <div>
                <Button
                  size="2"
                  variant="soft"
                  onClick={() => setTreeOpen((prev) => !prev)}
                >
                  {treeOpen ? (
                    <>
                      <EyeOff size={16} aria-hidden="true" />
                      {t('competencyMap.graphHideView')}
                    </>
                  ) : (
                    <>
                      <Eye size={16} aria-hidden="true" />
                      {t('competencyMap.graphToggleView')}
                    </>
                  )}
                </Button>
              </div>

              {treeOpen && (
                <SegmentedControl.Root
                  size="2"
                  value={mode}
                  onValueChange={(value) =>
                    setMode(value as 'canvas' | 'outline')
                  }
                  className={styles.graphMode}
                >
                  <SegmentedControl.Item value="canvas">
                    {t('competencyMap.graphView.modeCanvas')}
                  </SegmentedControl.Item>
                  <SegmentedControl.Item value="outline">
                    {t('competencyMap.graphView.modeOutline')}
                  </SegmentedControl.Item>
                </SegmentedControl.Root>
              )}

              {treeOpen && mode === 'canvas' && (
                <Suspense
                  fallback={
                    <Text as="p" role="status" color="gray" size="2">
                      {t('competencyMap.graphView.loading')}
                    </Text>
                  }
                >
                  <CourseGraphCanvas
                    graph={graph}
                    materialNames={materialNames}
                  />
                </Suspense>
              )}

              {treeOpen && mode === 'outline' && (
                <div className={styles.graphTree}>
                  <Text weight="bold" size="2">
                    {t('competencyMap.graphSections')}
                  </Text>
                  {(() => {
                    const topicsById = new Map(
                      graph.topics.map((t) => [t.id, t]),
                    )
                    const childrenByParent = new Map<string, string[]>()
                    const childIds = new Set<string>()

                    for (const h of graph.hierarchy) {
                      const list = childrenByParent.get(h.source) || []
                      list.push(h.target)
                      childrenByParent.set(h.source, list)
                      childIds.add(h.target)
                    }

                    const roots = graph.topics.filter(
                      (t) => !childIds.has(t.id),
                    )

                    const renderedIds = new Set<string>()

                    const renderSubtree = (
                      topicId: string,
                      visitedInBranch: Set<string>,
                    ): React.ReactNode => {
                      renderedIds.add(topicId)
                      const children = childrenByParent.get(topicId) || []
                      if (children.length === 0) return null

                      return (
                        <ul className={styles.graphChildrenList}>
                          {children.map((childId) => {
                            if (visitedInBranch.has(childId)) {
                              return null
                            }
                            const child = topicsById.get(childId)
                            const nextVisited = new Set(visitedInBranch).add(
                              childId,
                            )
                            return (
                              <li key={childId}>
                                <Text size="2" color="gray">
                                  └─ {child?.title || childId}
                                </Text>
                                {renderSubtree(childId, nextVisited)}
                              </li>
                            )
                          })}
                        </ul>
                      )
                    }

                    const primaryRoots = roots.length > 0 ? roots : graph.topics

                    return (
                      <div className={styles.graphSectionsList}>
                        {primaryRoots.map((root) => {
                          renderedIds.add(root.id)
                          return (
                            <div
                              key={root.id}
                              className={styles.graphSectionItem}
                            >
                              <Text weight="bold" size="2">
                                {root.title}
                              </Text>
                              {renderSubtree(root.id, new Set([root.id]))}
                            </div>
                          )
                        })}
                        {graph.topics
                          .filter((t) => !renderedIds.has(t.id))
                          .map((orphan) => {
                            renderedIds.add(orphan.id)
                            return (
                              <div
                                key={orphan.id}
                                className={styles.graphSectionItem}
                              >
                                <Text weight="bold" size="2">
                                  {orphan.title}
                                </Text>
                                {renderSubtree(orphan.id, new Set([orphan.id]))}
                              </div>
                            )
                          })}
                      </div>
                    )
                  })()}

                  {graph.prerequisites.length > 0 && (
                    <>
                      <Text
                        weight="bold"
                        size="2"
                        className={styles.graphDependenciesTitle}
                      >
                        {t('competencyMap.graphDependencies')}
                      </Text>
                      {(() => {
                        const topicsById = new Map(
                          graph.topics.map((t) => [t.id, t]),
                        )
                        return (
                          <div className={styles.graphPrereqList}>
                            {graph.prerequisites.map((p, idx) => {
                              const src =
                                topicsById.get(p.source)?.title || p.source
                              const tgt =
                                topicsById.get(p.target)?.title || p.target
                              return (
                                <div
                                  key={idx}
                                  className={styles.graphPrereqItem}
                                >
                                  <Text size="2" weight="bold">
                                    {src}
                                  </Text>
                                  <Text size="2" color="gray">
                                    →
                                  </Text>
                                  <Text size="2">{tgt}</Text>
                                  {p.reason && (
                                    <Text size="1" color="gray">
                                      ({p.reason})
                                    </Text>
                                  )}
                                </div>
                              )
                            })}
                          </div>
                        )
                      })()}
                    </>
                  )}
                </div>
              )}
            </>
          )}
        </div>
      )}
    </section>
  )
}
