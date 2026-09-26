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


> **Esta es la copia `bia-energy-live`**: el mismo proyecto más la **fase 7 del plan (streaming)**. Vive aparte para que el replay en vivo no afecte los tiempos ni la entrega principal (`bia-energy`). Todo lo demás es idéntico.

## Replay en vivo (streaming)

El dataset se reproduce hora a hora sobre un **reloj simulado**:

- Los días 1–6 se **precargan**; desde el día 7, cada hora llega como un evento (500 ms por hora a velocidad 1×; `STREAM_STEP_MS`).
- En cada hora el **motor por lotes** se vuelve a ejecutar sobre la ventana acumulada, con los eventos ya ocurridos (≈ 5 ms por ciclo). Es una desviación consciente del `Update` incremental del plan: el resultado final es por construcción el mismo que el del análisis por lotes, y un test lo verifica.
- **Gestor de alertas**: un hallazgo abre una alerta `CANDIDATE`; si persiste 3 h simuladas pasa a `CONFIRMED`; si cambia de tipo o severidad se publica `updated`; si desaparece, o el motor la reclasifica como falso positivo, se cierra (`CLOSED`) con el motivo.
- **Hub** con fan-out a todos los clientes y un buffer de los últimos 2.048 mensajes. Un cliente lento se desconecta y reanuda solo.

Secuencia sobre los CSV oficiales:

| Hora simulada | Cambio |
|---|---|
| D8 05:00 → 08:00 → 14:00 | M-106 candidata → confirmada (explicable) → cerrada como falso positivo al terminar la parada de 12 h |
| D11 05:00 → 08:00 | M-104 candidata → confirmada (explicable) |
| D12 19:00 → 22:00 | M-109 candidata → confirmada (anomalía real alta) |
| D13 04:00 → 07:00 | M-112 candidata → confirmada (calidad de datos) |

**API** (además de la del proyecto base):

| Ruta | Uso |
|---|---|
| `GET /api/stream?token=` | Server-Sent Events. Primero `snapshot` (reloj, medidores con sus últimas 48 lecturas, alertas); luego `tick`, `alert`, `control` y `snapshot` tras un reinicio. Con `Last-Event-ID` solo se reenvía lo perdido. El token va en la URL porque `EventSource` no envía cabeceras |
| `POST /api/stream/control` `{action, speed}` | `start`, `pause`, `reset`, `speed` (0,25–32) |
| `GET /api/stream/state` | Reloj y alertas actuales |

**Frontend**: pestaña **EN VIVO** con controles (iniciar, pausar, reiniciar, 1×–8×), reloj y avance, medidores con su curva de las últimas 48 h y su estado, alertas con su ciclo de vida y registro de cambios. En el header, el indicador LIVE con el reloj simulado; las alertas confirmadas, actualizadas y cerradas aparecen como notificaciones en cualquier pantalla. El estado vive en un store de Zustand alimentado por un único `EventSource`.

**Variables**: `STREAM_STEP_MS` (500), `STREAM_START_DAY` (7), `STREAM_AUTOSTART` (`false`).

**Tests**: consistencia batch vs. streaming en los dos datasets (mismos medidores, tipo y severidad; nada que el lote considere normal llega a confirmarse), M-109 confirmada ≥ 3 h después de su candidata, M-106 cerrada como falso positivo y nunca tratada como anomalía real, precarga, reanudación por `Last-Event-ID`, cliente lento, controles, SSE de punta a punta con `httptest`, y en el frontend los reducers, la página, las notificaciones y el hook con un `EventSource` simulado.

**Límites**: las alertas en vivo están en memoria y se reinician con el servidor; en un despliegue con varias réplicas cada una tendría su propio reloj.

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

### En producción

**Gratis (Render):** el `Dockerfile` de la raíz construye una sola imagen en la que la API de Go también sirve el frontend (`STATIC_DIR`). `render.yaml` la despliega en el plan gratuito con PostgreSQL gratuito: en Render, New → Blueprint → este repo, y se completan `ANTHROPIC_API_KEY` y `DEMO_PASSWORD`. En el plan gratuito el servicio se duerme tras 15 min sin tráfico y el primer acceso tarda ~1 min. La base gratuita vence a los 30 días; con `DB_FALLBACK_MEMORY=true` la app sigue funcionando en memoria.

De pago, con las imágenes separadas:

- **Railway**: un servicio por Dockerfile, configurado con `backend/railway.toml` y `frontend/railway.toml`, más PostgreSQL gestionado. En el servicio web, `API_UPSTREAM=api.railway.internal:8080`.
- **VPS**: `docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d --build`. Añade Caddy con HTTPS automático para el `DOMAIN` definido en `.env`.

`GET /api/health` indica qué motor de explicaciones está activo (`"ai":"claude:<modelo>"` o `"template"`).

### Probar la conexión con Claude

```bash
make ai-check    # lee ANTHROPIC_API_KEY de .env; corre el análisis y pide las 4 explicaciones a Claude
```

Cada hallazgo sale con `explained_by`. Si Claude falla o cita una cifra sin sustento, aparece una línea `FALLBACK` y ese hallazgo se explica con la plantilla.

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
| `ANTHROPIC_API_KEY` (alias `LLM_API_KEY`) | vacío | Activa las explicaciones redactadas por Claude |
| `ANTHROPIC_MODEL` (alias `LLM_MODEL`) | `claude-sonnet-5` | Modelo de Claude |
| `LLM_TIMEOUT_MS` | `10000` | Límite de una llamada a Claude; después se usa la plantilla |
| `TARIFF_COP_PER_KWH` | `850` | Tarifa para estimar el costo del impacto |
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

1. **Lecturas.** Cada lectura recibe banderas de calidad (se guardan con su hora y se ven en la UI):
   - `DQ_RANGE`: imposible, PF fuera de [0, 1] o 0 V con consumo;
   - `DQ_VOLTAGE`: voltaje fuera de la banda 209–231 V (220 V ± 5 %);
   - `DQ_JUMP`: salto de voltaje de más de 15 V entre horas seguidas;
   - `DQ_PF_JUMP`: el FP se aparta más de 0,15 de la mediana de sus vecinas (ventana de 7 h);
   - `DQ_PHYSICS`: la relación `k = kWh / (V·I·PF/1000)` se aparta más de ±25 % de la propia del medidor **y** de la de sus vecinas.

   La última condición es la clave: un cambio de régimen sostenido, como el de M-109, no se confunde con un error de datos. El inicio del problema es la primera hora con al menos 3 lecturas marcadas en 24 h, y la severidad es alta si más del 10 % de las lecturas de alguna ventana de 24 h quedan marcadas. La columna `status` del CSV se ignora.
2. **Baseline.** Mediana por hora del día de los días 1–7, más la MAD, sobre lecturas plausibles. Su suma es el **consumo esperado de un día**.
3. **Estado actual.** Consumo de las **últimas 24 h** frente al baseline diario. Es la cifra del ejemplo de la prueba («M-109 · 2.180 kWh · baseline ≈ 1.070 · +103,7 %»).
4. **Detección.** A partir del día 8, la desviación de cada hora contra su hora del baseline. Un **episodio** es una racha de al menos 6 h fuera de ±25 % (se toleran huecos de 2 h), con inicio, fin y recuperación. Así se detectan cambios que empiezan a mitad de semana (M-104 el día 11, M-109 el día 12) y paradas cortas (M-106, 12 h), que un promedio semanal diluiría. También se calculan picos por z-score robusto (> 3,5) y el patrón nocturno.
5. **Correlación eléctrica.** Corriente, voltaje, PF y la relación `kWh / (V·I·PF)`, comparando el baseline con la ventana del episodio.
6. **Eventos.** Cada evento tiene una categoría derivada de su tipo: `EXPLANATORY` (arranques, cambios operativos, paradas), `NON_EXPLANATORY` (`UNKNOWN`) o `INFORMATIONAL` (`DATA_QUALITY`). Un evento explica un episodio solo si:
   - es `EXPLANATORY`;
   - está a ±3 h del punto de cambio;
   - su efecto esperado coincide con la dirección del cambio (arranque ↔ aumento, parada ↔ caída);
   - si anuncia una duración («12 hours»), el episodio dura eso ±2 h.

   Un `UNKNOWN` **no explica** nada y un `DATA_QUALITY` solo corrobora; la clasificación nunca depende de su texto. La UI muestra cada evento evaluado con la razón.
7. **Clasificación.** El árbol de decisión aplica un orden fijo:
   1. **Calidad de datos**: ≥ 5 lecturas inconsistentes → `DATA_QUALITY`.
   2. **Evento coherente**: si es una parada y el consumo se recupera → `FALSE_POSITIVE`/`LOW`; si no → `EXPLAINABLE_ANOMALY`/`MEDIUM`.
   3. **Magnitud**: `REAL_ANOMALY`, con severidad `HIGH` si el desvío es > 50 % o hay señales eléctricas.
8. **Confianza, impacto y prioridad.**
   - `confianza = 0,35·magnitud + 0,25·persistencia + 0,25·variables que coinciden + 0,15·coherencia con eventos`, con tope 0,97. Cada componente se guarda con su detalle y la UI lo muestra como barra apilada.
   - **Impacto**: kWh extra por día y por mes, costo a la tarifa configurada y potencia reactiva (`tan(acos(FP))`, límite 0,5).
   - **Proyección 24 h**: perfil por hora del baseline × nivel observado desde el cambio (estacional ingenuo).
   - `prioridad = peso(severidad) × confianza × impacto normalizado`. Para calidad de datos el impacto es `0,5 + fracción de lecturas marcadas`.
   - El estado del medidor sale del análisis: falso positivo → `OK`, anomalía real alta → `CRITICAL`, el resto → `ALERT`.

El orden importa. Un medidor con lecturas inconsistentes no se confunde con un cambio de carga, y un evento coherente descarta la alarma antes de asignar severidad.

Los umbrales están en `analysis.DefaultConfig()` y el reporte los muestra en sus anexos. Los tests del motor corren sobre **dos datasets de forma distinta**: los CSV oficiales y un generador sintético con otros tiempos y otros tipos de error. En los dos deben salir los mismos cuatro veredictos, en el mismo orden.

### IA y explicabilidad (Claude)

- `explain.Claude` llama a la Messages API con **tool use forzado** (`report_explanation`). La salida es estructurada (`reason`, `evidence_summary`, `recommended_action`, `next_steps`) y no hay texto libre que parsear.
- Las explicaciones se piden en paralelo, con un límite de `LLM_TIMEOUT_MS` cada una, y se guardan en caché por un hash de la evidencia: repetir el análisis con los mismos datos no vuelve a llamar a Claude.
- **Validación de grounding:** cada número que cita Claude debe existir en la evidencia del motor. Si una cifra no coincide (`ErrUngrounded`), o si la API falla, `WithFallback` usa la plantilla determinista y lo registra. La UI indica si la explicación la redactó Claude o la plantilla del motor.
- Claude **no** cambia tipo, severidad ni cifras.
- La salida de cada anomalía sigue el formato pedido (`meter_id`, `anomaly`, `type`, `severity`, `confidence`, `reason`, `recommended_action`). Se puede ver en Investigación → «VER JSON».

---

## API

Todas las rutas van bajo `/api` y requieren `Authorization: Bearer <token>`, excepto `health` y `auth/login`.

| API mínima sugerida | Implementación | Notas |
|---|---|---|
| `GET /meters` | `GET /api/meters?status=&q=&sort=&order=` | `status`: `OK` / `ALERT` / `CRITICAL`; `q`: `meter_id`; `sort`: `severity` / `consumption` / `variation`; `order`: `desc` / `asc` |
| `GET /meters/:meterId` | `GET /api/meters/{meterId}` | Resumen, estadísticas, eventos y hallazgo IA |
| `GET /meters/:meterId/readings` | `GET /api/meters/{meterId}/readings?resolution=raw\|day&from=&to=` | 336 lecturas horarias o 14 puntos diarios; `from`/`to` en RFC 3339 o `YYYY-MM-DD` (`bucket` sigue aceptado) |
| `GET /anomalies` | `GET /api/anomalies?type=&severity=` | Ordenadas por prioridad |
| `GET /anomalies/:id` | `GET /api/anomalies/{id}` | Con serie diaria y perfil horario |
| `POST /ai/analyze` | `POST /api/ai/analyze` | `202` + `Location`; corre en segundo plano. Si ya hay uno en curso devuelve ese mismo (`202`) |
| `GET /ai/analysis/:id` | `GET /api/ai/analysis/{id\|latest}` | Estado de los 7 pasos y resumen |
| `GET /dashboard/summary` | `GET /api/dashboard/summary` | KPIs y último análisis |
| — | `PATCH /api/anomalies/{id}` `{status}` | Ciclo de vida: `OPEN → ACKNOWLEDGED → IN_PROGRESS → RESOLVED` |
| — | `GET /api/meters/{id}/events`, `GET /api/events` | Eventos operativos |
| — | `GET /api/reports/latest` | Reporte del último análisis |
| — | `GET /api/dashboard/heatmap` | Desviación diaria de cada medidor contra su baseline |
| — | `GET /api/meters/{id}/baseline` | Mediana, p10 y p90 por hora, k habitual, banda de voltaje y lecturas marcadas |
| — | `GET /api/meters/{id}/forecast` | Proyección de 24 h e impacto |
| — | `POST /api/anomalies/{id}/actions` `{action, note}` | `acknowledge`, `investigate`, `validate`, `resolve`, `dismiss` o `note`; mueve el estado y guarda el historial con el usuario del token |
| — | `POST /api/auth/login`, `GET /api/health` | Sesión y salud |

Los errores responden `{"error": {"code": "NOT_FOUND|INVALID|CONFLICT|UNAUTHORIZED|INTERNAL", "message": "…"}}`.

**Modelo de datos** (PostgreSQL, migraciones embebidas en el binario):

- `meters`, `readings` (PK `meter_id, ts`), `events`.
- `analysis_runs` (pasos con duración y resumen en JSONB).
- `anomalies` (evidencia, próximos pasos y `details`: desglose de confianza, impacto, punto de cambio, en JSONB).
- `anomaly_actions` (historial de acciones con nota, usuario y estado resultante).

---

## Frontend

| Pantalla | Qué responde |
|---|---|
| **Operación** (dashboard) | ¿Qué está pasando? KPIs, medidores agrupados por zona, foco con la explicación, pila de alarmas IA con su ciclo de vida, carga total más eventos y un mapa de calor medidor × día de la desviación contra el baseline |
| **Medidores** | Filtros (todos, normales, alertas, críticas), búsqueda por `meter_id`, orden por consumo, variación o severidad, y sparkline de 14 días |
| **Detalle** | Consumo actual contra baseline, variación, estado, histórico diario y horario, voltaje, corriente, PF, calidad de datos y veredicto IA |
| **Anomalías IA** | Tipo, severidad, confianza (Alta / Media / Baja y valor), razón, estado y acción, con filtros por tipo y severidad |
| **Investigación** | Qué encontró la IA y su evidencia; comparación contra el baseline: diaria, serie horaria con banda p10–p90, punto de cambio, eventos y proyección de 24 h, perfil horario y paneles de voltaje (209–231 V), corriente y FP (0,9); diagnóstico con k en el tiempo, corriente vs. consumo antes/después y energía acumulada real vs. esperada; eventos evaluados con la razón; desglose de la confianza; impacto (kWh, COP, reactiva); acciones con nota e historial; JSON |
| **Reporte IA** | Resumen ejecutivo, las 6 preguntas de la prueba, fichas de evidencia, plan de acción con checklist, anexos de metodología y trazabilidad. Vistas Completo y Ejecutivo; se exporta a PDF con la impresión del navegador |

**Run AI Analysis** está siempre en el header. La tira de progreso muestra los 7 pasos (Lecturas → Baseline → Detección → Correlación → Eventos → Explicación → Recomendación) con el resultado de cada uno, y termina en «4 anomalías detectadas · 2 requieren atención prioritaria».

---

## Tests

```bash
make test        # backend + frontend
make lint        # go vet, gofmt, oxlint, tsc
```

- **Backend** (`go test -race ./...`):
  - Motor: los 4 casos, el orden de prioridad y que no haya falsos positivos, sobre los CSV oficiales y sobre el dataset sintético; puntos de cambio (M-109 12/09 14:00, M-104 11/09 00:00, M-106 08/09 00:00–12:00); M-112 detectado sin eventos; banderas de calidad; duración de paradas; desglose de confianza e impacto; eventos `UNKNOWN`; estadística.
  - Explainer: mock HTTP de la API de Anthropic, rechazo de cifras no sustentadas y fallback.
  - Explainer: caché por evidencia, timeout y resumen de evidencia de la plantilla.
  - Servicios, HTTP (auth, filtros, orden, ventana de lecturas, formato de error, heatmap, baseline, proyección, acciones con el usuario del token, análisis en curso), CSV y dominio.
  - PostgreSQL: integración cuando existe `TEST_DATABASE_URL`; en CI corre con un servicio Postgres.
- **Frontend** (Vitest + Testing Library):
  - Formato es-CO, ciclo de vida, veredictos, geometría de gráficas, agrupación por zonas y narrativa del reporte.
  - Componentes: filtros, búsqueda y orden de Medidores contra la API; la tira del análisis; el polling hasta completar y la invalidación; login y rutas protegidas; Investigación (gráficas, desglose, impacto, acciones con nota, lecturas marcadas de M-112); mapa de calor; veredicto de cada evento.
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

## Guion de demo (5 min)

1. **Login** (`operador@vatio.demo` / `demo`) → Operación.
2. **Run AI Analysis**: la tira muestra los 7 pasos con su resultado y el tiempo de cómputo de cada uno; termina en «4 anomalías · 2 prioritarias».
3. **Mapa de calor**: M-109 en rojo desde D12, M-104 desde D11, M-106 azul solo en D8, M-112 con puntos violeta en D13–D14.
4. **P1 M-109** → Investigación: serie horaria fuera de la banda p10–p90 desde el 12/09 14:00, corriente ×2, FP 0,94 → 0,74, el evento `UNKNOWN` descartado con su razón, impacto ≈ 35.000 kWh/mes y reactiva sobre el límite. Registrar «Investigar» con una nota.
5. **P2 M-112**: k en el tiempo con los puntos rojos fuera de la banda y los saltos de voltaje: es un problema de medición, no de consumo.
6. **P3 M-104** y **P4 M-106**: el evento explica el cambio (dirección y, para la parada, duración de 12 h).
7. **Reporte IA**: resumen ejecutivo, las preguntas de la prueba, fichas y plan de acción; exportar a PDF.

## Decisiones y límites

- **Motor determinista + LLM redactor** en lugar de un LLM que clasifica. Es reproducible, auditable y barato, y la IA aporta la explicación sin inventar cifras.
- **Ejecución asíncrona** del análisis, con estado por pasos persistido: la UI hace polling cada 400 ms.
- **Desviaciones del plan de implementación** (documentadas):
  - No se usa CUSUM: el punto de cambio es el inicio del episodio horario (≥ 6 h fuera de ±25 %), que en los dos datasets coincide con la hora exacta del cambio.
  - No se implementa la regla `DQ_STUCK` (lecturas repetidas): en los CSV oficiales marcaba a los 12 medidores.
  - La ventana de eventos es ±3 h del punto de cambio (en lugar de ±24 h del inicio del día) y la de duración ±2 h.
  - La banda de `k` es relativa (±25 %) y exige apartarse de la propia **y** de la local, para no confundir un cambio de régimen con un error.
  - El streaming (fase 7) está en esta copia; el motor por lotes se vuelve a ejecutar sobre la ventana creciente en vez de un `Update` incremental.
- **Límites**:
  - Con 14 días de datos, el baseline usa 7 y no captura estacionalidad semanal.
  - Los eventos vienen en inglés y se citan tal como fueron registrados; Claude los traduce al redactar.
  - No hay topología eléctrica.
  - La autenticación es de demo: un único usuario configurado por entorno.
  - El checklist del plan en el reporte se guarda por navegador; el ciclo de vida de las anomalías sí se persiste en la API.
