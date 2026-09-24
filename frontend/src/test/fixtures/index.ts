// Real responses of the Go API over the synthetic dataset, captured after one analysis run.
import type { AnalysisRun, Anomaly, DashboardSummary, MeterSummary, Report } from '@/shared/api/types'
import anomaliesJson from './anomalies.json'
import metersJson from './meters.json'
import reportJson from './report.json'
import runJson from './run.json'
import summaryJson from './summary.json'

export const meters = metersJson as MeterSummary[]
export const anomalies = anomaliesJson as Anomaly[]
export const report = reportJson as unknown as Report
export const run = runJson as AnalysisRun
export const summary = summaryJson as unknown as DashboardSummary
