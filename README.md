# Vatio · AI Energy Management

MVP de la prueba técnica **AI Energy Management Platform**: una plataforma para gestionar medidores eléctricos donde la IA detecta, explica, prioriza y recomienda acciones sobre anomalías.

La pregunta que responde la aplicación es: **qué medidor revisar primero, y por qué.**

```
Login → Operación (dashboard) → M-109 → Run AI Analysis → Anomalía → Investigación → Acción → Reporte IA
```

Resultado sobre los **CSV oficiales** de la prueba (`backend/data`):

| Medidor | Qué encuentra el motor | Veredicto | Prioridad |
|---|---|---|---|
| M-109 | +110,7 % desde el día 12 14:00; corriente 200 → 425 A, PF 0,94 → 0,74; el único registro es `UNKNOWN` | `REAL_ANOMALY` · `HIGH` · conf 0,97 | **P1** |
| M-112 | Consumo estable (+0,5 %), pero desde el día 13 hay 17 lecturas inconsistentes: voltaje 202–241 V y saltos de FP | `DATA_QUALITY` · `HIGH` · conf 0,97 | P2 |
| M-104 | +47,4 % desde el día 11 00:00, coherente con «New production line activated» | `EXPLAINABLE_ANOMALY` · `MEDIUM` · conf 0,87 | P3 |
| M-106 | −80 % durante 12 h el día 8 y recuperación, coherente con «Scheduled maintenance outage» | `FALSE_POSITIVE` · `LOW` (`anomaly: false`) | P4 |

Los otros 8 medidores quedan en estado normal: no hay falsos positivos. Para verlo sin levantar nada: `cd backend && go run ./cmd/analyze -data data -v`.

---

## Ejecutar

### Con Docker (recomendado)

```bash
cp .env.example .env          # define AUTH_SECRET y, si quieres, ANTHROPIC_API_KEY
docker compose up --build     # o: make up
open http://localhost:8080    # operador@vatio.demo / demo
```

El stack levanta tres servicios:

- **db**: PostgreSQL 16.
- **api**: Go, en una imagen distroless que corre como usuario no root. No se publica fuera de la red de Docker.
- **web**: nginx sirve la SPA y hace de proxy de `/api` hacia la API.

Así el navegador habla con un solo origen y la `ANTHROPIC_API_KEY` nunca sale del backend. Al arrancar, la API aplica las migraciones y siembra la base desde los CSV si está vacía.

### En local (sin Docker)

Requisitos: Go ≥ 1.23 y Node ≥ 22.

```bash
make install
make dev-api    # API en :8080 con almacenamiento en memoria (sin DATABASE_URL)
make dev-web    # SPA en :5173; Vite hace proxy de /api hacia :8080
```

Para usar PostgreSQL en local: `DATABASE_URL=postgres://… make dev-api`.

### Variables de entorno (API)

| Variable | Por defecto | Uso |
|---|---|---|
| `PORT` | `8080` | Puerto HTTP |
| `DATABASE_URL` | vacío → memoria | DSN de PostgreSQL |
| `DATA_DIR` | `data` | Carpeta con `readings.csv`, `events.csv` y `meters.csv` (este último es opcional) |
| `ANTHROPIC_API_KEY` | vacío | Activa las explicaciones redactadas por Claude |
| `ANTHROPIC_MODEL` | `claude-sonnet-4-5` | Modelo de Claude |
| `AUTH_SECRET` | secreto de desarrollo (con aviso) | Secreto HMAC de los tokens |
| `DEMO_USER` / `DEMO_PASSWORD` | `operador@vatio.demo` / `demo` | Credenciales de la demo |
| `STEP_DELAY_MS` | `450` | Pausa entre pasos del análisis para que la UI muestre el progreso |

---

## Arquitectura

```
┌──────────────── frontend (React 19 + Vite + TS) ────────────────┐
│ features/  auth · operations · meters · anomalies · analysis ·   │
│            report                                                │
│ shared/    api (cliente tipado + TanStack Query) · charts (SVG)  │
│            · lib · ui                                            │
└───────────────────────────────┬──────────────────────────────────┘
                                │ /api (JSON, Bearer token)
┌───────────────────────────────▼──── backend (Go) ─────────────────┐
│ adapters   httpapi (net/http) · store/postgres · store/memory     │
│            · ingest (CSV)                                         │
│ app        casos de uso + puertos (repositorios, Explainer)       │
│ explain    Claude (tool use) · validación de cifras · plantilla   │
│ analysis   motor puro: baseline, detección, correlación, eventos, │
│            prioridad                                              │
│ domain     Meter · Reading · Event · Anomaly · AnalysisRun        │
└───────────────────────────────────────────────────────────────────┘
```

**Backend.** Es una arquitectura hexagonal y las dependencias apuntan hacia adentro:

- `domain` no importa nada de infraestructura.
- `analysis` es una función pura, `(medidores, lecturas, eventos) → hallazgos`. Es determinista y se prueba sin red ni base de datos.
- `app` orquesta los casos de uso contra interfaces (`MeterRepository`, `AnomalyRepository`, `Explainer`…).
- Los adaptadores son intercambiables: memoria o PostgreSQL, y Claude o la plantilla.

Solo se usa la librería estándar (`net/http` con el enrutado por patrones de Go 1.22) más `lib/pq`.

**Frontend.** Está organizado por *features*, y cada una es dueña de sus páginas, estado y lógica de presentación:

- El estado del servidor vive en TanStack Query, con claves centralizadas e invalidación al terminar un análisis o al cambiar el ciclo de vida de una anomalía.
- El estado de UI vive en Zustand: sesión, filtros compartidos entre Operación y Medidores, y la tira del análisis.
- Las gráficas son SVG propias: sin librerías, con la geometría en funciones puras probadas.
- Tailwind v4 lleva los tokens del sistema de diseño: la sala de control en oscuro y el reporte en papel claro.

```
backend/
  cmd/api          servidor HTTP (config por env, seed, graceful shutdown, healthcheck)
  cmd/gendata      genera el dataset sintético determinista (tests)
  cmd/analyze      corre el motor sobre una carpeta de CSV e imprime los hallazgos
  internal/domain  entidades, enums y máquina de estados de la anomalía
  internal/analysis  motor de anomalías (sin IO)
  internal/explain   Explainer: Claude, validación y plantilla
  internal/app       servicios, puertos, reporte
  internal/httpapi   handlers, auth, middleware
  internal/store     memory y postgres (migraciones embebidas)
  internal/ingest    lectura y escritura de CSV tolerante a formatos
frontend/src/
  app/             shell (header, banner P1, tira del análisis) y router
  features/        auth, operations, meters, anomalies, analysis, report
  shared/          api, charts, lib (formato es-CO, etiquetas, veredictos), ui
```

---

## Motor de análisis

La técnica es híbrida: la estadística robusta y las reglas de dominio **deciden**, y el LLM **redacta**. El tipo, la severidad, la confianza, la prioridad y cada cifra salen del motor. Claude solo convierte esa evidencia en lenguaje natural.

1. **Lecturas.** Cada lectura se valida contra el propio medidor y cuenta como **inconsistente** si:
   - es imposible: PF fuera de [0, 1], o 0 V con consumo;
   - el voltaje se aparta más de ±6 % del habitual del medidor;
   - el FP salta más de 0,15 frente a sus lecturas vecinas (ventana de 7 h);
   - la relación `kWh / (V·I·PF)` se aparta más de ±25 % de la propia del medidor **y** de la de sus vecinas.

   La última condición es la clave: un cambio de régimen sostenido, como el de M-109, no se confunde con un error de datos.
2. **Baseline.** Mediana por hora del día de los días 1–7, más la MAD, sobre lecturas plausibles. Su suma es el **consumo esperado de un día**.
3. **Estado actual.** Consumo de las **últimas 24 h** frente al baseline diario. Es la cifra del ejemplo de la prueba («M-109 · 2.180 kWh · baseline ≈ 1.070 · +103,7 %»).
4. **Detección.** A partir del día 8, la desviación de cada hora contra su hora del baseline. Un **episodio** es una racha de al menos 6 h fuera de ±25 % (se toleran huecos de 2 h), con inicio, fin y recuperación. Así se detectan cambios que empiezan a mitad de semana (M-104 el día 11, M-109 el día 12) y paradas cortas (M-106, 12 h), que un promedio semanal diluiría. También se calculan picos por z-score robusto (> 3,5) y el patrón nocturno.
5. **Correlación eléctrica.** Corriente, voltaje, PF y la relación `kWh / (V·I·PF)`, comparando el baseline con la ventana del episodio.
6. **Eventos.** Se cruza cada episodio con los eventos a ±24 h de su inicio, y el evento solo cuenta si su dirección es coherente: arranque o cambio operativo ↔ aumento, parada ↔ caída.
   - Un registro `UNKNOWN` («No operational event reported») **no explica** nada.
   - Un registro `DATA_QUALITY` refuerza la evidencia de medición.
7. **Clasificación.** El árbol de decisión aplica un orden fijo:
   1. **Calidad de datos**: ≥ 5 lecturas inconsistentes → `DATA_QUALITY`.
   2. **Evento coherente**: si es una parada y el consumo se recupera → `FALSE_POSITIVE`/`LOW`; si no → `EXPLAINABLE_ANOMALY`/`MEDIUM`.
   3. **Magnitud**: `REAL_ANOMALY`, con severidad `HIGH` si el desvío es > 50 % o hay señales eléctricas.
8. **Prioridad y confianza.**
   - `prioridad = peso(severidad) × magnitud × persistencia(h / 48) × (1 − 0,8 si un evento lo explica)`.
   - `confianza = clamp(0,5 + 0,07·señales + 0,2·fuerza, 0,5, 0,97)`.

El orden importa. Un medidor con lecturas inconsistentes no se confunde con un cambio de carga, y un evento coherente descarta la alarma antes de asignar severidad.

Los umbrales están en `analysis.DefaultConfig()` y el reporte los muestra en sus anexos. Los tests del motor corren sobre **dos datasets de forma distinta**: los CSV oficiales y un generador sintético con otros tiempos y otros tipos de error. En los dos deben salir los mismos cuatro veredictos, en el mismo orden.

### IA y explicabilidad (Claude)

- `explain.Claude` llama a la Messages API con **tool use forzado** (`report_explanation`). La salida es estructurada (`reason`, `recommended_action`, `next_steps`) y no hay texto libre que parsear.
- **Validación de grounding:** cada número que cita Claude debe existir en la evidencia del motor. Si una cifra no coincide (`ErrUngrounded`), o si la API falla, `WithFallback` usa la plantilla determinista y lo registra. La UI indica si la explicación la redactó Claude o la plantilla del motor.
- Claude **no** cambia tipo, severidad ni cifras.
- La salida de cada anomalía sigue el formato pedido (`meter_id`, `anomaly`, `type`, `severity`, `confidence`, `reason`, `recommended_action`). Se puede ver en Investigación → «VER JSON».

---

## API

Todas las rutas van bajo `/api` y requieren `Authorization: Bearer <token>`, excepto `health` y `auth/login`.

| API mínima sugerida | Implementación | Notas |
|---|---|---|
| `GET /meters` | `GET /api/meters?status=&q=&sort=` | `status`: `OK` / `ALERT` / `CRITICAL`; `q`: `meter_id`; `sort`: `severity` / `consumption` / `variation` |
| `GET /meters/:meterId` | `GET /api/meters/{meterId}` | Resumen, estadísticas, eventos y hallazgo IA |
| `GET /meters/:meterId/readings` | `GET /api/meters/{meterId}/readings?bucket=hour\|day` | 336 lecturas horarias o 14 puntos diarios |
| `GET /anomalies` | `GET /api/anomalies?type=&severity=` | Ordenadas por prioridad |
| `GET /anomalies/:id` | `GET /api/anomalies/{id}` | Con serie diaria y perfil horario |
| `POST /ai/analyze` | `POST /api/ai/analyze` | `202` + `Location`; corre en segundo plano |
| `GET /ai/analysis/:id` | `GET /api/ai/analysis/{id\|latest}` | Estado de los 7 pasos y resumen |
| `GET /dashboard/summary` | `GET /api/dashboard/summary` | KPIs y último análisis |
| — | `PATCH /api/anomalies/{id}` `{status}` | Ciclo de vida: `OPEN → ACKNOWLEDGED → IN_PROGRESS → RESOLVED` |
| — | `GET /api/meters/{id}/events`, `GET /api/events` | Eventos operativos |
| — | `GET /api/reports/latest` | Reporte del último análisis |
| — | `POST /api/auth/login`, `GET /api/health` | Sesión y salud |

**Modelo de datos** (PostgreSQL, migraciones embebidas en el binario):

- `meters`, `readings` (PK `meter_id, ts`), `events`.
- `analysis_runs` (pasos y resumen en JSONB).
- `anomalies` (evidencia y próximos pasos en JSONB).

---

## Frontend

| Pantalla | Qué responde |
|---|---|
| **Operación** (dashboard) | ¿Qué está pasando? KPIs, medidores agrupados por zona, foco con la explicación, pila de alarmas IA con su ciclo de vida, y carga total más eventos |
| **Medidores** | Filtros (todos, normales, alertas, críticas), búsqueda por `meter_id`, orden por consumo, variación o severidad, y sparkline de 14 días |
| **Detalle** | Consumo actual contra baseline, variación, estado, histórico diario y horario, voltaje, corriente, PF, calidad de datos y veredicto IA |
| **Anomalías IA** | Tipo, severidad, confianza, razón, estado y acción, con filtros por tipo y severidad |
| **Investigación** | Qué encontró la IA, las variables que cambiaron, la comparación contra el baseline (diaria, horaria o eléctrica), la evidencia, los eventos relacionados, la acción y el JSON |
| **Reporte IA** | Resumen ejecutivo, las 6 preguntas de la prueba, fichas de evidencia, plan de acción con checklist, anexos de metodología y trazabilidad. Vistas Completo y Ejecutivo; se exporta a PDF con la impresión del navegador |

**Run AI Analysis** está siempre en el header. La tira de progreso muestra los 7 pasos (Lecturas → Baseline → Detección → Correlación → Eventos → Explicación → Recomendación) con el resultado de cada uno, y termina en «4 anomalías detectadas · 2 requieren atención prioritaria».

---

## Tests

```bash
make test        # backend + frontend
make lint        # go vet, gofmt, oxlint, tsc
```

- **Backend** (`go test -race ./...`):
  - Motor: los 4 casos, el orden de prioridad y que no haya falsos positivos, sobre los CSV oficiales y sobre el dataset sintético; eventos `UNKNOWN`; estadística.
  - Explainer: mock HTTP de la API de Anthropic, rechazo de cifras no sustentadas y fallback.
  - Servicios, HTTP (auth, filtros, flujo de análisis, 404/409), CSV y dominio.
  - PostgreSQL: integración cuando existe `TEST_DATABASE_URL`; en CI corre con un servicio Postgres.
- **Frontend** (Vitest + Testing Library):
  - Formato es-CO, ciclo de vida, veredictos, geometría de gráficas, agrupación por zonas y narrativa del reporte.
  - Componentes: filtros, búsqueda y orden de Medidores contra la API; la tira del análisis; el polling hasta completar y la invalidación; login y rutas protegidas; Investigación con JSON.
  - Los fixtures son respuestas reales de la API.

CI (GitHub Actions) ejecuta lint, tests y build de ambos proyectos y construye las imágenes Docker.

---

## Datos

- **`backend/data`** contiene los CSV oficiales de la prueba: `readings.csv` (4.032 lecturas horarias de 12 medidores) y `events.csv` (4 registros).
  - El lector acepta su formato (`event_timestamp`, `event_type`, sin columna `id`), además de columnas en otro orden, alias, decimales con coma y varios formatos de fecha. Reporta los errores con número de línea.
  - `meters.csv` es un **catálogo de demo** con nombre y ubicación de cada medidor, porque el dataset solo trae `meter_id`. Es opcional: si falta, los medidores se derivan de las lecturas.
- **`cmd/gendata`** genera un dataset sintético determinista con los 4 casos, pero con otros tiempos (cambios desde el día 8, parada de 72 h, PF > 1 y 0 V). Se usa en los tests para comprobar que el motor no está ajustado a un solo dataset.
- **`expected_results.csv` no se usa en ningún punto.** El loader no lo lee; está en `.gitignore` y en `.dockerignore`, y no llega a la base, al prompt ni al usuario. Queda reservado al evaluador.

## Seguridad

- La `ANTHROPIC_API_KEY` solo existe en el backend, como variable de entorno. El SPA nunca la ve.
- Los tokens firmados con HMAC tienen expiración.
- La API no se publica en Docker; solo es accesible a través de nginx.
- nginx añade cabeceras de seguridad y la imagen de la API es distroless y no root.

## Decisiones y límites

- **Motor determinista + LLM redactor** en lugar de un LLM que clasifica. Es reproducible, auditable y barato, y la IA aporta la explicación sin inventar cifras.
- **Ejecución asíncrona** del análisis, con estado por pasos persistido: la UI hace polling cada 400 ms.
- **Límites**:
  - Con 14 días de datos, el baseline usa 7 y no captura estacionalidad semanal.
  - Los eventos vienen en inglés y se citan tal como fueron registrados; Claude los traduce al redactar.
  - No hay topología eléctrica.
  - La autenticación es de demo: un único usuario configurado por entorno.
  - El checklist del plan en el reporte se guarda por navegador; el ciclo de vida de las anomalías sí se persiste en la API.
