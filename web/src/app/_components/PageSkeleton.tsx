import styles from "./chrome.module.css";

type PageSkeletonProps = {
  cards?: number;
};

export default function PageSkeleton({ cards = 4 }: PageSkeletonProps) {
  return (
    <div aria-busy="true" aria-live="polite">
      <div className={styles.skeletonGrid}>
        {Array.from({ length: cards }, (_, index) => (
          <div className={styles.skeletonCard} key={index} />
        ))}
      </div>
      <div className={styles.skeletonBlock} />
    </div>
  );
}
