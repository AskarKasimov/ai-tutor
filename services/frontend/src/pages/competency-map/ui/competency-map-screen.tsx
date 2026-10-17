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
  Check,
  CheckCircle2,
  Clock,
  Download,
  Eye,
  EyeOff,
  FileText,
  ArrowRight,
  ShieldCheck,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  downloadCourseMaterialRaw,
  type CourseMaterial,
  type CourseMaterialKindOption,
} from '@/entities/competency-map'
import { CourseGraphApiError } from '@/entities/course-graph'
import { AccountMenu, useAuth } from '@/features/auth'
import {
  useDeleteCourseMaterialMutation,
  useSubjectMaterialsQuery,
  useCourseMaterialKindsQuery,
  useCourseMaterialsChanging,
  useReplaceCourseMaterialMutation,
  useUpdateCourseMaterialMetadataMutation,
  MaterialReorderControls,
  MaterialUploadQueueToolbar,
  MaterialUploadSubmit,
  MaterialQueueCard,
  CourseMaterialCard,
  useMaterialUploadQueue,
  moveMaterialQueueItem,
  moveMaterialQueueItemToIndex,
  sortMaterialsByWeek,
} from '@/features/import-competency-map'
import type { MaterialQueueItem } from '@/features/import-competency-map'
import {
  useCourseGraphQuery,
  useLatestCourseGraphBuildQuery,
  useStartCourseGraphBuildMutation,
} from '@/features/course-graph-build'
import {
  useCreateSubjectMutation,
  useSubjectsQuery,
} from '@/features/subject-selection'
import styles from '@/pages/competency-map/ui/competency-map.module.scss'

type MaterialListRow =
  | { id: string; week: string; queued: MaterialQueueItem }
  | { id: string; week: string; saved: CourseMaterial }

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
  const { t, i18n } = useTranslation()
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
  const materialKindsQuery = useCourseMaterialKindsQuery(userId)
  const materialKinds = materialKindsQuery.data ?? []
  const queue = useMaterialUploadQueue(userId, subjectId, materialKinds)
  const replaceMaterial = useReplaceCourseMaterialMutation(userId, subjectId)
  const updateMetadata = useUpdateCourseMaterialMetadataMutation(
    userId,
    subjectId,
  )
  const deleteMaterial = useDeleteCourseMaterialMutation(userId, subjectId)
  const latestBuild = useLatestCourseGraphBuildQuery(userId, subjectId)
  const courseGraph = useCourseGraphQuery(userId, subjectId)
  const startBuild = useStartCourseGraphBuildMutation(userId, subjectId)
  const [previewId, setPreviewId] = useState<string | null>(null)
  const [materialOrder, setMaterialOrder] = useState<string[]>([])
  const [draggingMaterialId, setDraggingMaterialId] = useState<string | null>(
    null,
  )
  const [dropTargetMaterialId, setDropTargetMaterialId] = useState<
    string | null
  >(null)

  // Covers queue uploads and saved-material operations.
  const materialsChanging = useCourseMaterialsChanging(userId, subjectId)
  const busy = create.isPending || materialsChanging
  const materials = materialsQuery.data ?? []
  const materialRows: MaterialListRow[] = [
    ...materials.map((saved) => ({
      id: saved.id,
      week: saved.week?.toString() ?? '',
      saved,
    })),
    ...queue.items.map((queued) => ({
      id: queued.id,
      week: queued.week,
      queued,
    })),
  ]
  const orderedMaterialRows = sortMaterialsByWeek(
    materialRows
      .map((row, index) => ({ ...row, index }))
      .sort((a, b) => {
        const aOrder = materialOrder.indexOf(a.id)
        const bOrder = materialOrder.indexOf(b.id)
        if (aOrder < 0 && bOrder < 0) return a.index - b.index
        if (aOrder < 0) return 1
        if (bOrder < 0) return -1
        return aOrder - bOrder
      }),
  )

  function applyMaterialOrder(
    next: Array<{ id: string; week: string }>,
    movedId: string,
  ) {
    setMaterialOrder(next.map((item) => item.id))
    queue.updateItems(next)
    const moved = next.find((item) => item.id === movedId)
    const previous = materials.find((item) => item.id === movedId)
    const week = Number(moved?.week)
    if (moved && previous && week > 0 && week !== previous.week)
      updateMetadata.mutate({
        materialId: movedId,
        patch: { week },
        subjectId,
      })
  }

  function moveMaterial(movedId: string, targetIndex: number) {
    const current = orderedMaterialRows.map(({ id, week }) => ({ id, week }))
    applyMaterialOrder(
      moveMaterialQueueItemToIndex(current, movedId, targetIndex),
      movedId,
    )
  }
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
    ...(materialsChanging ? [t('courseGraph.blockChanging')] : []),
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
                    disabled={busy}
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
                <span className={styles.stepNumber}>1</span>
                <Text weight="bold">{t('courseGraph.materialsStep')}</Text>
              </div>
              <ArrowRight className={styles.pipelineArrow} aria-hidden="true" />
              <div
                data-ready={
                  (parsedCount > 0 && graphBlockers.length === 0) ||
                  hasGraph ||
                  undefined
                }
              >
                <span className={styles.stepNumber}>2</span>
                <Text weight="bold">{t('courseGraph.graphStep')}</Text>
              </div>
              <ArrowRight className={styles.pipelineArrow} aria-hidden="true" />
              <div data-ready={hasGraph || undefined}>
                <span className={styles.stepNumber}>3</span>
                <Text weight="bold">{t('courseGraph.readyStep')}</Text>
              </div>
            </nav>

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

              {materialKindsQuery.isError && (
                <Flex gap="3" align="center" wrap="wrap">
                  <Text role="alert" color="red">
                    {t('courseGraph.kindsUnavailable')}
                  </Text>
                  <Button
                    variant="ghost"
                    size="1"
                    onClick={() => void materialKindsQuery.refetch()}
                  >
                    {t('trainer.retry')}
                  </Button>
                </Flex>
              )}
              {materialKindsQuery.isSuccess &&
                materialKindsQuery.data.length === 0 && (
                  <Text role="alert" color="amber">
                    {t('courseGraph.kindsEmpty')}
                  </Text>
                )}

              <MaterialUploadQueueToolbar queue={queue} disabled={busy} />

              {replaceMaterial.isError && (
                <Text as="p" role="alert" color="red">
                  {replaceMaterial.error instanceof Error
                    ? replaceMaterial.error.message
                    : t('competencyMap.requestError')}
                </Text>
              )}
              {materialsQuery.isPending && (
                <Text as="p" role="status" color="gray" size="2">
                  {t('competencyMap.loading')}
                </Text>
              )}
              {materialsQuery.isError && (
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
              )}
              {materialsQuery.isSuccess && orderedMaterialRows.length === 0 && (
                <Text as="p" color="gray" size="2">
                  {t('competencyMap.materialsEmpty')}
                </Text>
              )}
              {orderedMaterialRows.length > 0 && (
                <div className={styles.materialsList} role="list">
                  {orderedMaterialRows.map((row, index) => {
                    const name =
                      'queued' in row
                        ? row.queued.file.name
                        : row.saved.filename
                    return (
                      <div
                        className={styles.materialListItem}
                        key={row.id}
                        role="listitem"
                        draggable={!busy && !queue.preparing}
                        data-dragging={
                          draggingMaterialId === row.id || undefined
                        }
                        data-drop-target={
                          dropTargetMaterialId === row.id || undefined
                        }
                        onDragStart={(event) => {
                          const target = event.target
                          if (
                            target instanceof HTMLElement &&
                            target.closest(
                              'button, input, [role="combobox"]',
                            ) &&
                            !target.closest('[data-queue-drag-handle]')
                          ) {
                            event.preventDefault()
                            return
                          }
                          setDraggingMaterialId(row.id)
                          event.dataTransfer.effectAllowed = 'move'
                          event.dataTransfer.setData('text/plain', row.id)
                        }}
                        onDragEnd={() => {
                          setDraggingMaterialId(null)
                          setDropTargetMaterialId(null)
                        }}
                        onDragOver={(event) => {
                          event.preventDefault()
                          setDropTargetMaterialId(row.id)
                          event.dataTransfer.dropEffect = 'move'
                        }}
                        onDragLeave={() => setDropTargetMaterialId(null)}
                        onDrop={(event) => {
                          event.preventDefault()
                          const sourceId =
                            event.dataTransfer.getData('text/plain') ||
                            draggingMaterialId
                          if (sourceId) {
                            const current = orderedMaterialRows.map(
                              ({ id, week }) => ({ id, week }),
                            )
                            applyMaterialOrder(
                              moveMaterialQueueItem(current, sourceId, row.id),
                              sourceId,
                            )
                          }
                          setDraggingMaterialId(null)
                          setDropTargetMaterialId(null)
                        }}
                      >
                        <MaterialReorderControls
                          name={name}
                          disabled={busy || queue.preparing}
                          isFirst={index === 0}
                          isLast={index === orderedMaterialRows.length - 1}
                          onMoveUp={() => moveMaterial(row.id, index - 1)}
                          onMoveDown={() => moveMaterial(row.id, index + 1)}
                        />
                        {'queued' in row ? (
                          <MaterialQueueCard
                            item={row.queued}
                            kinds={materialKinds}
                            disabled={busy || queue.preparing}
                            dropTarget={dropTargetMaterialId === row.id}
                            onWeekChange={(week) =>
                              queue.updateWeek(row.id, week)
                            }
                            onKindChange={(kind) =>
                              queue.updateKind(row.id, kind)
                            }
                            onRemove={() => queue.removeItem(row.id)}
                          />
                        ) : (
                          <MaterialItemCard
                            material={row.saved}
                            kinds={materialKinds}
                            subjectId={subjectId}
                            dropTarget={dropTargetMaterialId === row.id}
                            previewOpen={previewId === row.id}
                            onTogglePreview={() =>
                              setPreviewId((current) =>
                                current === row.id ? null : row.id,
                              )
                            }
                            onDelete={() =>
                              deleteMaterial.mutateAsync({
                                materialId: row.id,
                                subjectId,
                              })
                            }
                            onReplace={(file) =>
                              replaceMaterial.mutate({
                                materialId: row.id,
                                file,
                              })
                            }
                            busy={busy}
                          />
                        )}
                      </div>
                    )
                  })}
                </div>
              )}
              <MaterialUploadSubmit queue={queue} disabled={busy} />
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
              {graphBlockers.length > 0 && (
                <div className={styles.warning} role="status">
                  <Text weight="bold">
                    {t(
                      hasGraph
                        ? 'courseGraph.rebuildBlockedTitle'
                        : 'courseGraph.blockedTitle',
                    )}
                  </Text>
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
              {latestBuild.data &&
                (latestBuild.data.startedAt !== null ||
                  latestBuild.data.completedAt !== null) && (
                  <Flex gap="2" wrap="wrap" direction="column">
                    {latestBuild.data.startedAt !== null && (
                      <Text size="1" color="gray">
                        {t('courseGraph.buildStartedAt', {
                          date: new Date(
                            latestBuild.data.startedAt * 1000,
                          ).toLocaleString(i18n.language),
                        })}
                      </Text>
                    )}
                    {latestBuild.data.completedAt !== null && (
                      <Text size="1" color="gray">
                        {t('courseGraph.buildCompletedAt', {
                          date: new Date(
                            latestBuild.data.completedAt * 1000,
                          ).toLocaleString(i18n.language),
                        })}
                      </Text>
                    )}
                  </Flex>
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
                    {courseGraph.data.publishedAt !== null && (
                      <span className={styles.graphPublishedAt}>
                        {t('courseGraph.graphPublishedAt', {
                          date: new Date(
                            courseGraph.data.publishedAt * 1000,
                          ).toLocaleString(i18n.language),
                        })}
                      </span>
                    )}
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
                        disabled={materialsChanging}
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
  kinds,
  subjectId,
  dropTarget,
  previewOpen,
  onTogglePreview,
  onDelete,
  onReplace,
  busy,
}: {
  material: CourseMaterial
  kinds: CourseMaterialKindOption[]
  subjectId: string
  dropTarget: boolean
  previewOpen: boolean
  onTogglePreview: () => void
  onDelete: () => Promise<void>
  onReplace: (file: File) => void
  busy: boolean
}) {
  const { t, i18n } = useTranslation()
  const [downloading, setDownloading] = useState(false)
  const [downloadError, setDownloadError] = useState<string | null>(null)
  const [actionError, setActionError] = useState<{
    message: string
    retry: () => Promise<unknown>
  } | null>(null)
  async function runDelete() {
    try {
      await onDelete()
      setActionError(null)
    } catch (error) {
      setActionError({
        message:
          error instanceof Error
            ? error.message
            : t('competencyMap.requestError'),
        retry: onDelete,
      })
    }
  }
  async function retryAction() {
    if (!actionError) return
    try {
      await actionError.retry()
      setActionError(null)
    } catch (error) {
      setActionError({
        ...actionError,
        message:
          error instanceof Error
            ? error.message
            : t('competencyMap.requestError'),
      })
    }
  }
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
    <CourseMaterialCard
      name={material.filename}
      status={t(`competencyMap.materialsStatus.${material.status}`)}
      statusColor={statusTone}
      dropTarget={dropTarget}
      meta={
        <>
          <span>
            {t('competencyMap.materialsSize', { size: formattedSize })}
          </span>
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
        </>
      }
      actions={
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
                  <Button color="red" onClick={() => void runDelete()}>
                    {t('competencyMap.confirm')}
                  </Button>
                </AlertDialog.Action>
              </Flex>
            </AlertDialog.Content>
          </AlertDialog.Root>
        </Flex>
      }
    >
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
          <Text as="span" size="2">
            {t('courseGraph.weekLabel')}
          </Text>
          <Text as="span" size="2" weight="medium">
            {material.week ?? '—'}
          </Text>
        </label>
        {material.kind && (
          <Badge color="gray" size="2">
            {kinds.find((kind) => kind.code === material.kind)?.label ??
              material.kind}
          </Badge>
        )}
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
      {actionError && (
        <Flex gap="2" align="center" wrap="wrap">
          <Text as="p" role="alert" color="red" size="2">
            {actionError.message}
          </Text>
          <Button size="1" variant="soft" onClick={() => void retryAction()}>
            {t('trainer.retry')}
          </Button>
        </Flex>
      )}

      {previewOpen && (
        <div className={styles.materialPreview}>
          {material.parsedText || '(текст отсутствует)'}
        </div>
      )}
    </CourseMaterialCard>
  )
}
