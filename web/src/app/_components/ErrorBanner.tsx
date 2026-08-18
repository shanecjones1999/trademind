import styles from "./chrome.module.css";

type ErrorBannerProps = {
  message: string;
  onRetry?: () => void;
};

export default function ErrorBanner({ message, onRetry }: ErrorBannerProps) {
  return (
    <div className={styles.banner} role="alert">
      <p>{message}</p>
      {onRetry ? (
        <button className={styles.retry} onClick={onRetry} type="button">
          Retry
        </button>
      ) : null}
    </div>
  );
}
