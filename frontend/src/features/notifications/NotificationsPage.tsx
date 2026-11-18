import React, { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCheck, Plus, Send, Trash2 } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import {
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  Modal,
  Skeleton,
  StatusBadge,
  inputClassName,
} from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import type { NotificationChannel } from "@/types/api";
import { formatRelativeTime } from "@/utils/format";

export const NotificationsPage: React.FC = () => {
  const { canManageInfra, isDemoMode } = useAuth();
  const { notify } = useToast();
  const queryClient = useQueryClient();

  const [severity, setSeverity] = useState("");
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [channelModal, setChannelModal] = useState(false);
  const [deleteCh, setDeleteCh] = useState<NotificationChannel | null>(null);

  const [chName, setChName] = useState("");
  const [chType, setChType] = useState<"email" | "slack" | "webhook">("slack");
  const [chRef, setChRef] = useState("env:SLACK_WEBHOOK_URL");
  const [chSummary, setChSummary] = useState("#infra-alerts");

  const notifsQ = useQuery({
    queryKey: ["notifications", severity, unreadOnly, isDemoMode],
    queryFn: () => api.listNotifications({ severity, unread_only: unreadOnly }),
  });

  const channelsQ = useQuery({
    queryKey: ["notification-channels", isDemoMode],
    queryFn: () => api.listNotificationChannels(),
  });

  const readMut = useMutation({
    mutationFn: (id: string) => api.markNotificationRead(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["notifications"] });
      void queryClient.invalidateQueries({ queryKey: ["unread-notifications"] });
    },
  });

  const readAllMut = useMutation({
    mutationFn: () => api.markAllNotificationsRead(),
    onSuccess: () => {
      notify({ type: "success", title: "All notifications marked as read" });
      void queryClient.invalidateQueries({ queryKey: ["notifications"] });
      void queryClient.invalidateQueries({ queryKey: ["unread-notifications"] });
    },
  });

  const createChMut = useMutation({
    mutationFn: () =>
      api.createNotificationChannel({
        name: chName,
        channel_type: chType,
        configuration_reference: chRef,
        target_summary: chSummary,
        severity_filters: ["critical", "warning"],
        enabled: true,
      }),
    onSuccess: () => {
      notify({ type: "success", title: "Notification channel configured" });
      setChannelModal(false);
      setChName("");
      void queryClient.invalidateQueries({ queryKey: ["notification-channels"] });
    },
    onError: (err) => notify({ type: "error", title: "Invalid channel configuration", description: err instanceof Error ? err.message : "" }),
  });

  const testChMut = useMutation({
    mutationFn: (id: string) => api.testNotificationChannel(id),
    onSuccess: () => {
      notify({ type: "success", title: "Test notification sent through Alertmanager" });
      void queryClient.invalidateQueries({ queryKey: ["notification-channels"] });
      void queryClient.invalidateQueries({ queryKey: ["notifications"] });
    },
    onError: (err) => notify({ type: "error", title: "Test failed", description: err instanceof Error ? err.message : "" }),
  });

  const delChMut = useMutation({
    mutationFn: (id: string) => api.deleteNotificationChannel(id),
    onSuccess: () => {
      notify({ type: "info", title: "Notification channel removed" });
      setDeleteCh(null);
      void queryClient.invalidateQueries({ queryKey: ["notification-channels"] });
    },
  });

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Notifications Center</h1>
          <p className="text-xs text-slate-500">
            Delivery history and Alertmanager notification channels · Secrets are referenced via env:/secret: keys
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" onClick={() => readAllMut.mutate()}>
            <CheckCheck className="h-4 w-4" /> Mark All Read
          </Button>
          {canManageInfra && (
            <Button size="sm" onClick={() => setChannelModal(true)}>
              <Plus className="h-4 w-4" /> Add Channel
            </Button>
          )}
        </div>
      </div>

      {/* Configured Notification Channels */}
      <Card title="Notification Channels (Alertmanager Routing)" subtitle="Email, Slack, and Generic Webhook destinations">
        {channelsQ.isLoading ? (
          <Skeleton className="h-28 w-full" />
        ) : !(channelsQ.data?.length) ? (
          <EmptyState title="No notification channels" description="Configure an Email, Slack, or Webhook channel backed by Alertmanager." />
        ) : (
          <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {channelsQ.data.map((ch) => (
              <div key={ch.id} className="rounded-lg border border-slate-200 dark:border-slate-800 p-3.5 text-xs space-y-2">
                <div className="flex items-start justify-between">
                  <div>
                    <p className="font-semibold text-sm">{ch.name}</p>
                    <p className="text-slate-500 uppercase">{ch.channel_type} · {ch.target_summary}</p>
                  </div>
                  <StatusBadge status={ch.enabled ? "UP" : "UNKNOWN"} />
                </div>
                <p className="font-mono text-[11px] text-slate-500 truncate">Ref: {ch.configuration_reference}</p>
                <p className="text-[11px] text-slate-500">
                  Last status: <span className="font-medium">{ch.last_status}</span>
                  {ch.last_tested_at && ` (${formatRelativeTime(ch.last_tested_at)})`}
                </p>
                {canManageInfra && (
                  <div className="flex items-center justify-between pt-2 border-t border-slate-100 dark:border-slate-800">
                    <Button size="sm" variant="secondary" onClick={() => testChMut.mutate(ch.id)} disabled={testChMut.isPending}>
                      <Send className="h-3 w-3" /> Test via Alertmanager
                    </Button>
                    <button type="button" onClick={() => setDeleteCh(ch)} aria-label="Delete channel" className="text-rose-500 p-1 hover:bg-rose-50 rounded">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* Notification History */}
      <Card
        title="Notification History"
        action={
          <div className="flex items-center gap-2 text-xs">
            <select value={severity} onChange={(e) => setSeverity(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5">
              <option value="">All Severities</option>
              <option value="critical">Critical</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
              <option value="resolved">Resolved</option>
            </select>
            <label className="inline-flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" checked={unreadOnly} onChange={(e) => setUnreadOnly(e.target.checked)} />
              Unread only
            </label>
          </div>
        }
      >
        {notifsQ.isLoading ? (
          <Skeleton className="h-40 w-full" />
        ) : !(notifsQ.data?.items?.length) ? (
          <EmptyState title="No notifications" description="Alert deliveries, agent disconnections, and system events will appear here." />
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {notifsQ.data.items.map((n) => (
              <li key={n.id} className={`py-3 flex items-start justify-between gap-3 text-xs ${!n.is_read ? "bg-brand-50/40 dark:bg-brand-950/20 -mx-5 px-5" : ""}`}>
                <div className="space-y-1">
                  <div className="flex items-center gap-2 flex-wrap">
                    <StatusBadge status={n.severity} />
                    <span className="font-semibold text-sm text-slate-900 dark:text-white">{n.title}</span>
                    {!n.is_read && <span className="h-2 w-2 rounded-full bg-brand-600" aria-label="Unread" />}
                  </div>
                  <p className="text-slate-600 dark:text-slate-300">{n.message}</p>
                  <div className="flex items-center gap-3 text-[11px] text-slate-400">
                    <span>{formatRelativeTime(n.attempted_at)}</span>
                    <span>Status: {n.status}</span>
                    {n.alert_fingerprint && (
                      <Link to={`/alerts/${encodeURIComponent(n.alert_fingerprint)}`} className="text-brand-600 hover:underline">
                        Open related alert →
                      </Link>
                    )}
                  </div>
                </div>
                {!n.is_read && (
                  <Button size="sm" variant="ghost" onClick={() => readMut.mutate(n.id)}>
                    Mark read
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>

      <Modal
        open={channelModal}
        onClose={() => setChannelModal(false)}
        title="Add Notification Channel"
        description="Use environment/secret references (e.g. env:SLACK_WEBHOOK_URL) rather than plaintext webhook secrets."
      >
        <form onSubmit={(e) => { e.preventDefault(); createChMut.mutate(); }} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Channel Name *</label>
            <input required value={chName} onChange={(e) => setChName(e.target.value)} placeholder="Production Ops Slack" className={inputClassName} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Channel Type *</label>
            <select value={chType} onChange={(e) => setChType(e.target.value as "email" | "slack" | "webhook")} className={inputClassName}>
              <option value="slack">Slack Incoming Webhook</option>
              <option value="email">Email</option>
              <option value="webhook">Generic HTTPS Webhook</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Configuration / Secret Reference *</label>
            <input required value={chRef} onChange={(e) => setChRef(e.target.value)} placeholder="env:SLACK_WEBHOOK_URL or ops@example.com" className={`${inputClassName} font-mono text-xs`} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Target Display Summary</label>
            <input value={chSummary} onChange={(e) => setChSummary(e.target.value)} placeholder="#infra-alerts" className={inputClassName} />
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setChannelModal(false)}>Cancel</Button>
            <Button type="submit" disabled={createChMut.isPending}>Save Channel</Button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteCh)}
        onClose={() => setDeleteCh(null)}
        onConfirm={() => deleteCh && delChMut.mutate(deleteCh.id)}
        title="Delete Notification Channel?"
        message={`Remove notification channel "${deleteCh?.name}"?`}
        confirmLabel="Delete"
        loading={delChMut.isPending}
      />
    </div>
  );
};
