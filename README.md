# Agente local de impresión

Requiere Windows. El instalador y el agente compilados funcionan sin instalar
Go en la PC del local y no ejecutan comandos de shell para imprimir.

## Instalación como servicio

1. En la web entra como ADMIN a **Impresión → Agentes**, crea un agente y copia su token.
2. Abre `FoodTruckPrinterSetup.exe` con doble clic y acepta el aviso de administrador de Windows.
3. En el formulario del navegador pega la URL de NestJS, incluido el prefijo
   (por ejemplo `https://tu-dominio/api` o `http://localhost:8000/api`), y el token.
   Marca **Guardar tickets como TXT** únicamente para una prueba con un agente dedicado.
4. Pulsa **Instalar o actualizar servicio**. Busca **Food Truck Printer** en
   la app Servicios de Windows y revisa el estado del agente en la web.

El formulario usa un endpoint temporal en `127.0.0.1` con una ruta aleatoria y
se cierra al completar la instalación. El servicio arranca automáticamente
con inicio diferido, bajo la cuenta **LocalSystem**. Para Winspool, instala
una cola visible para esa cuenta. Las impresoras TCP usan la IP y puerto
configurados en la API. El instalador no está firmado digitalmente.

La configuración queda en `%ProgramFiles%\Food Truck Printer\data\config.json`.
El JSON contiene la URL, el modo de simulación y el token protegido por
DPAPI. La carpeta está restringida a SYSTEM y administradores. En la misma
carpeta están `agent.log`, `state` y, en simulación, `out`.
Para actualizar o cambiar la configuración, abre el instalador nuevamente.
Puedes dejar el token vacío si conservas la misma URL. Si rotaste el token
en NestJS, pega el nuevo. El instalador conserva el directorio del diario.

## Reconstruir el instalador

Desde la raíz del repositorio, en Windows con Go 1.26 o posterior:

```text
go run ./cmd/build-installer
```

El compilador genera el agente, lo incrusta en el instalador y deja
`dist/FoodTruckPrinterSetup.exe`. El archivo resultante es autónomo.
La reconstrucción no registra ni ejecuta el servicio.

## Modo desarrollo

1. Inicia sesión como ADMIN y crea un agente con `POST /api/printer-agents` y `{"name":"Caja local"}`. Guarda el token devuelto.
2. Crea una impresora con `POST /api/printers`. Para simular usa `{"name":"Caja","purpose":"CASHIER","connectionType":"NETWORK_TCP","host":"127.0.0.1","port":9100,"printerAgentId":"UUID_DEL_AGENTE"}`.
3. Arranca el agente en PowerShell.

```powershell
$env:PRINTER_API_URL = 'http://127.0.0.1:8000/api'
$env:PRINTER_AGENT_TOKEN = 'TOKEN_DEL_AGENTE'
$env:PRINTER_OUTPUT_DIR = './out'
go run ./cmd/agent
```

En desarrollo, `go run ./cmd/agent` también lee automáticamente el archivo
`.env` de esta carpeta. Las variables ya definidas en PowerShell tienen
prioridad. El instalador no usa ese archivo: guarda la configuración protegida
en `config.json`.

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

El agente mantiene además una conexión Socket.IO v4 con el namespace
`/printer-worker`. El servidor solo envía `printer.jobs.available` como aviso;
el agente reclama el trabajo por HTTP con su token Bearer. Si el WebSocket se
cae, el agente conserva un polling de respaldo cada 60 segundos cuando no hay
trabajos y vuelve a reconectar automáticamente.

Para usar un ejecutable de desarrollo independiente del instalador, compila
con `go build -o agent.exe ./cmd/agent`. En ejecución interactiva recibe
la configuración por variables de entorno.

## API local de descubrimiento

Mientras el agente está ejecutándose, expone únicamente en `127.0.0.1` la API
local `http://127.0.0.1:17890`. No es un servidor público ni requiere el token
del agente para las consultas locales.

```text
GET  /health
POST /printers/discover
GET  /printers/check?host=192.168.1.50&port=9100
```

`/printers/discover` recibe un JSON obligatorio y solo revisa la red indicada:

```json
{"network":"192.168.101.x","port":9100}
```

También acepta CIDR, por ejemplo `192.168.101.0/24`. Prueba TCP con
concurrencia limitada y enumera las colas locales y conectadas de Windows. El
puerto local puede cambiarse en modo interactivo con
`PRINTER_LOCAL_ADDR=127.0.0.1:17891`.

## Recuperación de intentos

El agente guarda el intento antes de imprimir y el resultado antes de enviarlo
a NestJS. Si se pierde la conexión, conserva la confirmación y la reenvía hasta
que la API la acepte. Al reiniciar, termina esa confirmación antes de tomar
otro trabajo. Una interrupción sin resultado guardado se reporta como fallo
incierto, sin reimpresión automática. Si NestJS devuelve 409 por un intento
obsoleto, queda un archivo `review-*.json` para diagnóstico.

En modo interactivo, el diario se guarda bajo `%LOCALAPPDATA%\food-truck-printer\state`, separado
por URL de API. Puedes fijar otra ruta persistente con `PRINTER_STATE_DIR`.
Conserva la ruta, la URL y la cuenta de Windows al reiniciar. El bloqueo local
impide ejecutar dos instancias sobre el mismo diario. El token no se guarda
allí.

Si se pierde la respuesta de la reserva antes de que el agente reciba su ID,
el trabajo puede seguir en `PROCESSING` en la API. Revisa la cola y la salida
antes de volver a generarlo.
