# Agente local de impresión

Requiere Windows y Go 1.22 o posterior. Ejecuta desde esta carpeta. El agente
no necesita dependencias Go adicionales ni ejecuta comandos de shell para imprimir.

1. Inicia sesión como ADMIN y crea un agente con `POST /api/printer-agents` y `{"name":"Caja local"}`. Guarda el token devuelto.
2. Crea una impresora con `POST /api/printers`. Para simular usa `{"name":"Caja","purpose":"CASHIER","connectionType":"NETWORK_TCP","host":"127.0.0.1","port":9100,"printerAgentId":"UUID_DEL_AGENTE"}`.
3. Arranca el agente en PowerShell.

```powershell
$env:PRINTER_API_URL = 'http://127.0.0.1:8000/api'
$env:PRINTER_AGENT_TOKEN = 'TOKEN_DEL_AGENTE'
$env:PRINTER_OUTPUT_DIR = './out'
go run ./cmd/agent
```

4. Envía `POST /api/printers/:id/test` con `{}` y consulta el trabajo en `GET /api/print-jobs/:id`. En modo archivo, `PRINTED` confirma la escritura del TXT en `out`. Usa un agente dedicado para simular.

Para impresión física, quita `PRINTER_OUTPUT_DIR` y configura el destino real.
`NETWORK_TCP` usa el host y puerto configurados. `SYSTEM_PRINTER` usa el
nombre exacto de una cola instalada en Windows y envía bytes RAW vía Winspool.
Ambos transportes requieren una impresora compatible con ESC/POS. El formato
usa CP850 (tabla 2), líneas de 32/48 caracteres para papel de 58/80 mm y corte
parcial. Valida estos valores con el modelo real. `PRINTED` confirma el envío
completo al transporte, y en modo TXT confirma la escritura del archivo.

Para comandas específicas asigna el `productionAreaId` de cocina o barra.
Usa HTTPS si el agente se conecta por una red no confiable.

Para distribuir el agente, compila en Windows con
`go build -o agent.exe ./cmd/agent`. La aplicación recibe la misma configuración
por variables de entorno. El servicio automático de Windows se puede añadir
en una entrega posterior.

El agente reintenta confirmar el resultado sin repetir la impresión. La API permite reintentar únicamente los fallos confirmados antes del envío. Si el agente pierde la conexión o se apaga durante un trabajo, este puede quedar en `PROCESSING`. Revisa la salida antes de generar otro trabajo.
