import {
  AlertDialog,
  Badge,
  Button,
  Flex,
  Heading,
  Progress,
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
  Upload,
  X,
} from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { CourseMaterial } from '@/entities/competency-map'
import { useAuth } from '@/features/auth'
import {
  useSubjectMaterialsQuery,
  useUploadCourseMaterialsMutation,
  useDeleteCourseMaterialMutation,
} from '@/features/import-competency-map'
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
  const [createdId, setCreatedId] = useState('')
  const subject = subjects.data?.[0]
  const subjectId = subject?.id ?? createdId

  const materialsQuery = useSubjectMaterialsQuery(userId, subjectId)
  const uploadMaterials = useUploadCourseMaterialsMutation(userId, subjectId)
  const deleteMaterial = useDeleteCourseMaterialMutation(userId, subjectId)

  const [subjectName, setSubjectName] = useState('')
  const [lectureFiles, setLectureFiles] = useState<File[]>([])
  const [materialsFileError, setMaterialsFileError] = useState<string | null>(
    null,
  )
  const [previewId, setPreviewId] = useState<string | null>(null)

  const busy =
    create.isPending || uploadMaterials.isPending || deleteMaterial.isPending

  function selectLectureFiles(selected: FileList | null) {
    uploadMaterials.reset()
    setMaterialsFileError(null)
    if (!selected || selected.length === 0) return
    const validFiles: File[] = []
    for (let i = 0; i < selected.length; i++) {
      const f = selected[i]
      if (!/\.(pdf|md|txt|markdown)$/i.test(f.name)) {
        setMaterialsFileError(t('competencyMap.materialsInvalidType'))
        return
      }
      if (f.size > 50 * 1024 * 1024) {
        setMaterialsFileError(t('competencyMap.tooLarge'))
        return
      }
      if (f.size === 0) {
        setMaterialsFileError(t('competencyMap.emptyFile'))
        return
      }
      validFiles.push(f)
    }
    setLectureFiles((prev) => [...prev, ...validFiles])
  }

  function removeLectureFile(index: number) {
    setLectureFiles((prev) => prev.filter((_, i) => i !== index))
  }

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
        </div>

        {subjects.isPending ? (
          <Text as="p" role="status" color="gray">
            {t('competencyMap.loading')}
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
                  onSuccess: (created) => {
                    setCreatedId(created.id)
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

              <div className={styles.sectionHeading}>
                <BookOpen aria-hidden="true" />
                <Heading as="h3" size="4">
                  {t('competencyMap.materialsDropTitle')}
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

              {lectureFiles.length > 0 && (
                <div className={styles.actions}>
                  <Button
                    size="3"
                    color="green"
                    className={styles.submit}
                    disabled={lectureFiles.length === 0 || busy}
                    onClick={() => {
                      uploadMaterials.mutate(
                        { files: lectureFiles, subjectId },
                        { onSuccess: () => setLectureFiles([]) },
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

  const rawUrl = `/api/v1/admin/subjects/${encodeURIComponent(subjectId)}/materials/${encodeURIComponent(material.id)}/raw`

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

          <a href={rawUrl} target="_blank" rel="noopener noreferrer" download>
            <Button size="1" variant="ghost" color="gray">
              <Download size={14} />
            </Button>
          </a>

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
