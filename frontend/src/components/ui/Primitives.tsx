import React, { useEffect } from "react";
import {
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Clock,
  HelpCircle,
  Inbox,
  RefreshCw,
  X,
  XCircle,
} from "lucide-react";
import { cn } from "@/utils/format";

// Reusable Button
interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "sm" | "md" | "lg";
}

export const Button: React.FC<ButtonProps> = ({
  variant = "primary",
  size = "md",
  className,
  children,
  ...rest
}) => {
  const variants = {
    primary:
      "bg-brand-600 text-white hover:bg-brand-700 disabled:bg-brand-400 shadow-sm",
    secondary:
      "bg-white dark:bg-navy-800 text-slate-700 dark:text-slate-200 border border-slate-300 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-navy-700",
    danger:
      "bg-rose-600 text-white hover:bg-rose-700 disabled:bg-rose-400 shadow-sm",
    ghost:
      "text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-navy-800",
  };
  const sizes = {
    sm: "px-2.5 py-1.5 text-xs gap-1.5",
    md: "px-3.5 py-2 text-sm gap-2",
    lg: "px-5 py-2.5 text-sm gap-2",
  };
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center rounded-lg font-medium transition-colors disabled:opacity-60 disabled:cursor-not-allowed",
        variants[variant],
        sizes[size],
        className
      )}
      {...rest}
    >
      {children}
    </button>
  );
};

// Reusable Card
export const Card: React.FC<{
  title?: string;
  subtitle?: string;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}> = ({ title, subtitle, action, children, className }) => (
  <section
    className={cn(
      "rounded-xl border border-slate-200/80 dark:border-slate-800 bg-white dark:bg-navy-900 shadow-sm",
      className
    )}
  >
    {(title || action) && (
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 dark:border-slate-800 px-5 py-3.5">
        <div>
          {title && (
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
              {title}
            </h2>
          )}
          {subtitle && (
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-0.5">
              {subtitle}
            </p>
          )}
        </div>
        {action && <div className="flex items-center gap-2">{action}</div>}
      </div>
    )}
    <div className="p-5">{children}</div>
  </section>
);

// Status Badge — communicates status with BOTH icon and text (never color alone!)
export const StatusBadge: React.FC<{ status: string; isStale?: boolean; className?: string }> = ({
  status,
  isStale,
  className,
}) => {
  const s = (status || "UNKNOWN").toUpperCase();
  const styles: Record<string, { bg: string; icon: React.ReactNode; label: string }> = {
    UP: {
      bg: "bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/50 dark:text-emerald-300 dark:border-emerald-800",
      icon: <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Healthy",
    },
    HEALTHY: {
      bg: "bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/50 dark:text-emerald-300 dark:border-emerald-800",
      icon: <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Healthy",
    },
    RESOLVED: {
      bg: "bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/50 dark:text-emerald-300 dark:border-emerald-800",
      icon: <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Resolved",
    },
    WARNING: {
      bg: "bg-amber-50 text-amber-800 border-amber-200 dark:bg-amber-950/50 dark:text-amber-300 dark:border-amber-800",
      icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Warning",
    },
    CRITICAL: {
      bg: "bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/50 dark:text-rose-300 dark:border-rose-800",
      icon: <XCircle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Critical",
    },
    DOWN: {
      bg: "bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/50 dark:text-rose-300 dark:border-rose-800",
      icon: <XCircle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Down",
    },
    UNAVAILABLE: {
      bg: "bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/50 dark:text-rose-300 dark:border-rose-800",
      icon: <XCircle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Unavailable",
    },
    PENDING: {
      bg: "bg-slate-100 text-slate-700 border-slate-300 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700",
      icon: <Clock className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Pending",
    },
    INFO: {
      bg: "bg-sky-50 text-sky-700 border-sky-200 dark:bg-sky-950/50 dark:text-sky-300 dark:border-sky-800",
      icon: <AlertCircle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Info",
    },
    ACTIVE: {
      bg: "bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-950/50 dark:text-rose-300 dark:border-rose-800",
      icon: <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Active",
    },
    UNKNOWN: {
      bg: "bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:border-slate-700",
      icon: <HelpCircle className="h-3.5 w-3.5" aria-hidden="true" />,
      label: "Unknown",
    },
  };
  const cfg = styles[s] ?? styles.UNKNOWN;
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        className={cn(
          "inline-flex items-center gap-1 rounded-full border px-2.5 py-0.5 text-xs font-medium",
          cfg.bg,
          className
        )}
      >
        {cfg.icon}
        {cfg.label}
      </span>
      {isStale && (
        <span className="inline-flex items-center gap-1 rounded-full border border-amber-300 bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:border-amber-700 dark:bg-amber-950/50 dark:text-amber-300">
          <Clock className="h-3 w-3" aria-hidden="true" />
          Stale
        </span>
      )}
    </span>
  );
};

// Accessible Reusable Modal Dialog
export const Modal: React.FC<{
  open: boolean;
  onClose: () => void;
  title: string;
  description?: string;
  size?: "sm" | "md" | "lg";
  children: React.ReactNode;
}> = ({ open, onClose, title, description, size = "md", children }) => {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  const widths = { sm: "max-w-md", md: "max-w-xl", lg: "max-w-3xl" };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/60 p-4 backdrop-blur-xs"
      role="dialog"
      aria-modal="true"
      aria-labelledby="modal-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        className={cn(
          "w-full rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-900 shadow-2xl max-h-[90vh] flex flex-col",
          widths[size]
        )}
      >
        <div className="flex items-start justify-between border-b border-slate-200 dark:border-slate-800 px-5 py-4">
          <div>
            <h3 id="modal-title" className="text-base font-semibold text-slate-900 dark:text-slate-100">
              {title}
            </h3>
            {description && (
              <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{description}</p>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close modal"
            className="rounded-lg p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-navy-800 dark:hover:text-slate-200"
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <div className="overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>
  );
};

// Reusable Confirmation Dialog (never use window.confirm!)
export const ConfirmDialog: React.FC<{
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: string;
  message: string;
  confirmLabel?: string;
  loading?: boolean;
}> = ({ open, onClose, onConfirm, title, message, confirmLabel = "Confirm", loading }) => (
  <Modal open={open} onClose={onClose} title={title} size="sm">
    <p className="text-sm text-slate-600 dark:text-slate-300">{message}</p>
    <div className="mt-5 flex justify-end gap-2">
      <Button variant="secondary" onClick={onClose} disabled={loading}>
        Cancel
      </Button>
      <Button variant="danger" onClick={onConfirm} disabled={loading}>
        {loading ? "Processing..." : confirmLabel}
      </Button>
    </div>
  </Modal>
);

// Skeleton Loading States
export const Skeleton: React.FC<{ className?: string }> = ({ className }) => (
  <div
    aria-hidden="true"
    className={cn("animate-pulse rounded-md bg-slate-200/80 dark:bg-slate-800", className)}
  />
);

// Empty State
export const EmptyState: React.FC<{
  title: string;
  description: string;
  action?: React.ReactNode;
  icon?: React.ReactNode;
}> = ({ title, description, action, icon }) => (
  <div className="flex flex-col items-center justify-center rounded-xl border border-dashed border-slate-300 dark:border-slate-700 bg-slate-50/50 dark:bg-navy-900/40 px-6 py-12 text-center">
    <div className="mb-3 rounded-full bg-slate-100 dark:bg-navy-800 p-3 text-slate-500 dark:text-slate-400">
      {icon ?? <Inbox className="h-6 w-6" aria-hidden="true" />}
    </div>
    <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h3>
    <p className="mt-1 max-w-md text-xs text-slate-500 dark:text-slate-400">{description}</p>
    {action && <div className="mt-4">{action}</div>}
  </div>
);

// Error State
export const ErrorState: React.FC<{
  title?: string;
  message: string;
  onRetry?: () => void;
}> = ({ title = "Unable to load monitoring data", message, onRetry }) => (
  <div
    role="alert"
    className="flex flex-col items-center justify-center rounded-xl border border-rose-200 dark:border-rose-900/70 bg-rose-50/60 dark:bg-rose-950/20 px-6 py-10 text-center"
  >
    <XCircle className="h-8 w-8 text-rose-500 mb-2" aria-hidden="true" />
    <h3 className="text-sm font-semibold text-rose-900 dark:text-rose-200">{title}</h3>
    <p className="mt-1 max-w-md text-xs text-rose-700 dark:text-rose-300">{message}</p>
    {onRetry && (
      <Button variant="secondary" size="sm" className="mt-4" onClick={onRetry}>
        <RefreshCw className="h-3.5 w-3.5" /> Retry
      </Button>
    )}
  </div>
);

// React Error Boundary
interface ErrorBoundaryState {
  hasError: boolean;
  message: string;
}

export class ErrorBoundary extends React.Component<{ children: React.ReactNode }, ErrorBoundaryState> {
  constructor(props: { children: React.ReactNode }) {
    super(props);
    this.state = { hasError: false, message: "" };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, message: error.message || "Unexpected application error" };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="min-h-screen flex items-center justify-center p-6 bg-slate-50 dark:bg-navy-950">
          <ErrorState
            title="Something went wrong in CloudPulse"
            message={this.state.message}
            onRetry={() => {
              this.setState({ hasError: false, message: "" });
              window.location.reload();
            }}
          />
        </div>
      );
    }
    return this.props.children;
  }
}

export const inputClassName =
  "w-full rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-navy-800 px-3 py-2 text-sm text-slate-900 dark:text-slate-100 placeholder:text-slate-400 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-500/20";
