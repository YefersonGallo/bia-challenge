import { createBrowserRouter, Navigate } from 'react-router-dom'
import { LoginPage } from '@/features/auth/LoginPage'
import { RequireAuth } from '@/features/auth/RequireAuth'
import { OperationsPage } from '@/features/operations/OperationsPage'
import { MetersPage } from '@/features/meters/MetersPage'
import { MeterDetailPage } from '@/features/meters/MeterDetailPage'
import { AnomaliesPage } from '@/features/anomalies/AnomaliesPage'
import { InvestigationPage } from '@/features/anomalies/InvestigationPage'
import { ReportPage } from '@/features/report/ReportPage'
import { AppShell } from './AppShell'

/** Route table: Operación → Medidores → Detalle → Anomalías IA → Investigación → Reporte. */
export const routes = [
  { path: '/login', element: <LoginPage /> },
  {
    path: '/',
    element: (
      <RequireAuth>
        <AppShell />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <OperationsPage /> },
      { path: 'meters', element: <MetersPage /> },
      { path: 'meters/:meterId', element: <MeterDetailPage /> },
      { path: 'anomalies', element: <AnomaliesPage /> },
      { path: 'anomalies/:id', element: <InvestigationPage /> },
      { path: 'report', element: <ReportPage /> },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
]

export const createRouter = () => createBrowserRouter(routes)
