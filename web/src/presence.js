const onlineWindowMs = 20_000;

// presence describes whether an agent is still polling and when it last was.
export function presence(iso, now = Date.now()) {
  if (!iso) return { online: false, label: "Never seen", when: "" };
  const then = new Date(iso);
  const secs = Math.max(0, Math.round((now - then.getTime()) / 1000));
  const when = then.toLocaleString(undefined, {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
  if (now - then.getTime() < onlineWindowMs) return { online: true, label: "Online", when };
  let rel;
  if (secs < 60) rel = `${secs}s ago`;
  else if (secs < 3600) rel = `${Math.floor(secs / 60)}m ago`;
  else if (secs < 86400) rel = `${Math.floor(secs / 3600)}h ago`;
  else rel = `${Math.floor(secs / 86400)}d ago`;
  return { online: false, label: `Offline · ${rel}`, when };
}
