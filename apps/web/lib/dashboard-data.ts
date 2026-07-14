export const summary = [
  { label: "Servers", value: "12", detail: "9 online" },
  { label: "Warnings", value: "2", detail: "CPU and disk" },
  { label: "Offline", value: "1", detail: "heartbeat missed" },
  { label: "Alerts", value: "4", detail: "active incidents" },
];

export const servers = [
  { name: "prod-api-01", host: "api-01.nox.internal", env: "production", status: "online", cpu: 34, mem: 53 },
  { name: "worker-eu-02", host: "wrk-02.nox.internal", env: "production", status: "warning", cpu: 91, mem: 72 },
  { name: "staging-db-01", host: "db-stg-01.nox.internal", env: "staging", status: "offline", cpu: 0, mem: 0 },
] as const;
