import {
  AlertDialog,
  Badge,
  Button,
  Callout,
  Flex,
  Heading,
  Text,
} from '@radix-ui/themes'
import { useNavigate } from '@tanstack/react-router'
import {
  BookOpen,
  CheckCircle2,
  Clock,
  Download,
  Eye,
  EyeOff,
  FileText,
  Layers,
  Network,
  ShieldCheck,
  Target,
  Trash2,
  TriangleAlert,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  downloadCourseMaterialRaw,
  type CourseGraphSnapshot,
  type CourseMaterial,
} from '@/entities/competency-map'
import { AccountMenu, useAuth } from '@/features/auth'
import {
  materialInProgress,
  useCourseMaterialsChanging,
  useBuildCourseGraphMutation,
  useCourseGraphQuery,
  useDeleteCourseMaterialMutation,
  useSubjectMaterialsQuery,
  MaterialUploadQueue,
} from '@/features/import-competency-map'
import { useSubjectsQuery } from '@/features/subject-selection'
import styles from '@/pages/competency-map/ui/competency-map.module.scss'

export function CompetencyMapAdminScreen({ subjectId }: { subjectId: string }) {
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
  return (
    <CompetencyMapAdmin
      key={subjectId}
      userId={user.id}
      subjectId={subjectId}
    />
  )
}

function CompetencyMapAdmin({
  userId,
  subjectId,
}: {
  userId: string
  subjectId: string
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const subjects = useSubjectsQuery(userId)
  const subject = subjects.data?.find((item) => item.id === subjectId)
  const materialsQuery = useSubjectMaterialsQuery(userId, subjectId)
  const materials = materialsQuery.data ?? []
  const materialsBusy = materials.some(materialInProgress)
  const graphQuery = useCourseGraphQuery(userId, subjectId, materialsBusy)
  const buildGraph = useBuildCourseGraphMutation(userId, subjectId)
  const deleteMaterial = useDeleteCourseMaterialMutation(userId, subjectId)
  const [previewId, setPreviewId] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState('')
  const refetchGraph = graphQuery.refetch
  useEffect(() => {
    if (!materialsBusy) void refetchGraph()
  }, [materialsBusy, refetchGraph])
  const materialsChanging =
    useCourseMaterialsChanging(userId, subjectId) || deleteMaterial.isPending
  const graphReady = !!graphQuery.data?.graph && !graphQuery.data.stale
  return (
    <div className={styles.page} data-admin-screen>
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
            {t('courseGraph.subtitle')}
          </Text>
        </div>
        <Button
          variant="ghost"
          onClick={() => void navigate({ to: '/admin/courses' })}
        >
          {t('courseGraph.backToCourses')}
        </Button>
        {subjects.isPending ? (
          <Text role="status">{t('competencyMap.subjectsLoading')}</Text>
        ) : subjects.isError ? (
          <div className={styles.card}>
            <Text role="alert">{t('competencyMap.subjectsError')}</Text>
            <Button onClick={() => void subjects.refetch()}>
              {t('trainer.retry')}
            </Button>
          </div>
        ) : !subject ? (
          <div className={styles.card}>
            <Text role="alert">{t('courseGraph.courseNotFound')}</Text>
          </div>
        ) : (
          <>
            <section className={styles.card}>
              <Text color="gray">{t('competencyMap.subject')}</Text>
              <Heading as="h2" size="6">
                {subject.name}
              </Heading>
            </section>
            <nav
              className={styles.pipeline}
              aria-label={t('courseGraph.pipeline')}
            >
              <div data-active="true">
                <span className={styles.stepNumber}>1</span>
                <Text weight="bold">{t('courseGraph.materialsStep')}</Text>
              </div>
              <div data-ready={graphQuery.data?.inputsReady || undefined}>
                <span className={styles.stepNumber}>2</span>
                <Text weight="bold">{t('courseGraph.graphStep')}</Text>
              </div>
              <div data-ready={graphReady || undefined}>
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
              <MaterialUploadQueue
                userId={userId}
                subjectId={subjectId}
                disabled={deleteMaterial.isPending}
              />
              {deleteError && (
                <Text role="alert" color="red">
                  {deleteError}
                </Text>
              )}
              {materialsQuery.isPending ? (
                <Text role="status">{t('competencyMap.loading')}</Text>
              ) : materialsQuery.isError ? (
                <Flex gap="3">
                  <Text role="alert">{t('competencyMap.loadError')}</Text>
                  <Button onClick={() => void materialsQuery.refetch()}>
                    {t('trainer.retry')}
                  </Button>
                </Flex>
              ) : materials.length === 0 ? (
                <Text color="gray">{t('competencyMap.materialsEmpty')}</Text>
              ) : (
                <div className={styles.materialsList}>
                  {materials.map((material) => (
                    <MaterialItemCard
                      key={material.id}
                      material={material}
                      subjectId={subjectId}
                      busy={deleteMaterial.isPending}
                      previewOpen={previewId === material.id}
                      onTogglePreview={() =>
                        setPreviewId((current) =>
                          current === material.id ? null : material.id,
                        )
                      }
                      onDelete={() =>
                        deleteMaterial.mutate(
                          { materialId: material.id, subjectId },
                          {
                            onSuccess: () => setDeleteError(''),
                            onError: (error) => setDeleteError(error.message),
                          },
                        )
                      }
                    />
                  ))}
                </div>
              )}
            </section>
            <CourseGraphCard
              snapshot={graphQuery.data}
              loading={graphQuery.isPending}
              error={graphQuery.isError}
              onRetry={() => void graphQuery.refetch()}
              onBuild={() => buildGraph.mutate()}
              buildPending={
                buildGraph.isPending ||
                materialsChanging ||
                materialsQuery.isPending ||
                materialsQuery.isError ||
                materialsBusy
              }
              buildError={
                buildGraph.isError ? buildGraph.error.message : undefined
              }
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

function CourseGraphCard({
  snapshot,
  loading,
  error,
  onRetry,
  onBuild,
  buildPending,
  buildError,
}: {
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
            <AlertDialog.Root>
              <AlertDialog.Trigger>
                <Button
                  size="2"
                  color="green"
                  disabled={
                    !snapshot?.inputsReady || isBuilding || buildPending
                  }
                >
                  <Network size={16} aria-hidden="true" />
                  {t('competencyMap.graphBuild')}
                </Button>
              </AlertDialog.Trigger>
              <AlertDialog.Content maxWidth="440px">
                <AlertDialog.Title>
                  {t(
                    graph
                      ? 'courseGraph.rebuildTitle'
                      : 'courseGraph.buildTitle',
                  )}
                </AlertDialog.Title>
                <AlertDialog.Description>
                  {t(
                    graph
                      ? 'courseGraph.rebuildWarning'
                      : 'courseGraph.buildConfirm',
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
                      disabled={
                        !snapshot?.inputsReady || isBuilding || buildPending
                      }
                      onClick={onBuild}
                    >
                      {t('courseGraph.confirmBuild')}
                    </Button>
                  </AlertDialog.Action>
                </Flex>
              </AlertDialog.Content>
            </AlertDialog.Root>
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
