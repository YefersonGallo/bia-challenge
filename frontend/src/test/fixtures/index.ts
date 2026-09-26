// Real responses of the Go API over the official dataset (backend/data), captured after one analysis run.
import type {
  AnalysisRun,
  Anomaly,
  AnomalyDetail,
  Baseline,
  DashboardSummary,
  Forecast,
  HeatmapRow,
  MeterSummary,
  Reading,
  Report,
} from '@/shared/api/types'
import anomaliesJson from './anomalies.json'
import detailJson from './anomaly-detail.json'
import detailM112Json from './anomaly-detail-m112.json'
import baselineJson from './baseline.json'
import forecastJson from './forecast.json'
import heatmapJson from './heatmap.json'
import metersJson from './meters.json'
import readingsJson from './readings-m109.json'
import reportJson from './report.json'
import runJson from './run.json'
import summaryJson from './summary.json'

export const meters = metersJson as MeterSummary[]
export const anomalies = anomaliesJson as Anomaly[]
export const report = reportJson as unknown as Report
export const run = runJson as AnalysisRun
export const summary = summaryJson as unknown as DashboardSummary
export const heatmap = heatmapJson as unknown as HeatmapRow[]
/** M-109 (real anomaly) and M-112 (data quality) as returned by GET /anomalies/{id}. */
export const detailM109 = detailJson as unknown as AnomalyDetail
export const detailM112 = detailM112Json as unknown as AnomalyDetail
export const baselineM109 = baselineJson as unknown as Baseline
export const forecastM109 = forecastJson as unknown as Forecast
/** M-109 readings of days 10–14. */
export const readingsM109 = readingsJson as Reading[]
