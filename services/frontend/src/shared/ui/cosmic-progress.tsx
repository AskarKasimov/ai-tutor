import styles from './cosmic-progress.module.scss'

// A progress bar in deep space: a glowing violet-to-teal comet flies over a
// starry track. A null value shows an indeterminate sweep without a percent.
export function CosmicProgress({
  value,
  label,
  valueText,
}: {
  value: number | null
  label: string
  valueText?: string
}) {
  const percent =
    value === null ? null : Math.min(100, Math.max(0, Math.round(value)))
  return (
    <div
      className={styles.track}
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={percent ?? undefined}
      aria-valuetext={valueText}
      aria-busy={percent === null || undefined}
    >
      <span className={styles.stars} aria-hidden="true" />
      {percent === null ? (
        <span className={styles.sweep} aria-hidden="true" />
      ) : (
        <span
          className={styles.fill}
          style={{ width: `${percent}%` }}
          aria-hidden="true"
        >
          {percent > 0 && <span className={styles.comet} />}
        </span>
      )}
    </div>
  )
}
