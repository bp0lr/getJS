# Plan de mejoras de getJS

Estado: plan en curso. Requisito de Go y módulos actualizados; cambios funcionales pendientes. `main.go` todavía no fue modificado.

## Diagnóstico

Revisión inicial del checkout `e607fc6`, realizada el 7 de septiembre de 2026. Originalmente contenía `main.go` y `README.md`, sin módulos, tests ni CI. Go local original: `go1.25.6 windows/amd64`; el primer `go test ./...` falló por falta del módulo.

Actualización: agregados `go.mod` y `go.sum`, con mínimo Go 1.26.0 y toolchain preferido Go 1.27.1. Dependencias directas fijadas: goquery v1.13.0, pflag v1.0.10 y aurora v2.0.3+incompatible, conservando el import existente de aurora. Build y vet verificados con Go 1.26.0 y Go 1.27.1 en Windows; ayuda verificada con 1.27.1. `go test ./...` termina correctamente en ambas versiones pero todavía no hay tests.

La elección del mínimo es una política de mantenimiento: Go mantiene las dos ramas mayores más recientes, actualmente 1.26 y 1.27. Para uso diario, elegir el último parche de una rama soportada. No se exige 1.27 como mínimo por una necesidad del código. El toolchain seleccionado se descarga mediante Go sin reemplazar la instalación global.

GitHub confirmó la cuenta `bp0lr`, acceso de escritura a `bp0lr/getJS`, rama principal `master` y ningún issue o PR abierto al momento de la revisión. La identidad Git local es independiente: actualmente figura `bp0` con correo `bp0@lovan.cc`.

Los hallazgos provienen de lectura del código. Todavía no hay mediciones de rendimiento ni reproducciones automatizadas.

| Prioridad | Hallazgo | Consecuencia | Propuesta |
| --- | --- | --- | --- |
| Resuelto | Faltaban `go.mod` y `go.sum`. | Compilación no reproducible. | Módulo `github.com/bp0lr/getJS` y dependencias fijadas; Go 1.26.0 como mínimo y 1.27.1 preferido. |
| P0 | `completeUrls` concatena rutas y accede a `s[i][1]`. | URLs incorrectas; una referencia `/` puede provocar un panic. | Usar `url.ResolveReference`, la URL final y el primer `<base href>` válido. |
| P0 | Se ignora el error de `url.Parse(urlArg)`. | Posible panic al usar `DomainFolder.Host`; agrupación incorrecta con múltiples páginas. | Validar entradas y calcular el directorio por página. |
| P0 | `timeOutArg` queda en cero. | Peticiones sin plazo máximo efectivo. | Timeout finito configurable y cancelación con contexto. |
| P0 | `InsecureSkipVerify` siempre activo. | No se validan certificados TLS. | Validar por defecto y ofrecer una opción explícita para desactivarlo. |
| P1 | Un cliente y transporte por petición. | No se aprovecha un pool compartido de conexiones. | Compartir cliente y gestionar cuerpos y conexiones. |
| P1 | Peticiones secuenciales y sin deduplicación. | Latencia acumulada y solicitudes repetidas. | Concurrencia acotada, límites por host y deduplicación con orden estable. |
| P1 | `--resolve` usa GET sin necesidad de guardar. | Solicita contenido innecesario; cerrar sin consumir puede impedir reutilizar conexiones. | Medir HEAD con fallback frente a GET con lectura acotada. |
| P1 | Headers con `strings.Split` y logs de valores completos. | Valores con `:` se pierden; credenciales en logs. | Separar por el primer `:`, validar y ocultar valores sensibles. |
| P1 | Checks y descargas no reciben headers personalizados. | Fallan scripts que requieren autenticación. | Propagar en el mismo origen y excluir credenciales al cambiar de origen, también en redirects. |
| P1 | `saveJS` ignora errores y maneja mal las colisiones. | Parciales, sobrescrituras y errores silenciosos. | Escritura temporal, nombres estables y creación sin sobrescritura accidental. |
| P1 | Logs, errores y resultados se mezclan o silencian. | Pipelines poco fiables y códigos de salida engañosos. | stdout para resultados, stderr para diagnóstico y códigos documentados. |
| P2 | Entradas y salida a archivo se acumulan en slices. | Memoria proporcional al lote, además del DOM HTML. | Procesamiento por flujo y escritor con buffer. |
| P2 | La detección inline está dentro del bucle de atributos. | Texto inline puede desplazar un `data-src` válido. | Resolver atributos externos antes de clasificar inline. |
| P2 | Variables sin uso y estado global. | Tests difíciles de aislar y configuración confusa. | Separar configuración, extracción, HTTP y escritura en archivos del mismo paquete. |

## Entrega 1: compilación y corrección

1. Módulo y dependencias fijadas: completado. Verificar también instalación desde un checkout limpio durante CI.
2. Separar extracción y resolución de URLs de la CLI, conservando una estructura pequeña.
3. Corregir rutas relativas, referencias vacías, queries, fragments, base HTML y redirects. Procesar para red solo esquemas HTTP y HTTPS.
4. Validar entradas. Manejar errores de stdin sin usar un `FileInfo` nulo, recortar líneas y omitir vacías.
5. Agregar tests de extracción, resolución, entrada y clasificación inline.

Cierre: `go build ./...`, `go test ./...` y `go vet ./...` pasan. Las pruebas usan HTML local y `httptest`, sin sitios externos.

## Entrega 2: HTTP y errores

1. Implementar timeout finito, cancelación y verificación TLS por defecto.
2. Corregir headers y definir su propagación por origen, incluyendo redirects. No registrar valores sensibles.
3. Separar stdout/stderr y documentar códigos para argumentos inválidos, errores operativos y fallos parciales.
4. Validar configuración del proxy y devolver errores útiles.
5. Conservar los defaults actuales de `--complete` y `--resolve`. Documentar el cambio de comportamiento TLS.

Cierre: tests locales cubren servidor lento, cancelación, TLS, redirects entre orígenes, headers con dos puntos y errores de entrada/salida.

## Entrega 3: rendimiento medido

1. Registrar una base de comparación una vez que el programa compile.
2. Compartir cliente y transporte, con límites de conexiones y manejo explícito de cuerpos.
3. Agregar trabajadores acotados y límites por host. Mantener salida determinista mediante buffers acotados y contrapresión.
4. Deduplicar páginas y referencias antes de solicitar recursos. Considerar el contexto de autenticación; conservar queries porque pueden identificar recursos distintos.
5. Escribir resultados progresivamente y eliminar `allSources`. Documentar que la deduplicación exacta todavía consume memoria proporcional a las URLs únicas.
6. Comparar HEAD con fallback frente a GET. HEAD puede no estar soportado o comportarse distinto: decidir a partir de mediciones y tests.
7. Evaluar un tokenizer HTML solo si los perfiles muestran que el DOM es un costo relevante.

Medición: servidor HTTP local con varias páginas, scripts compartidos, latencia controlada y cuerpos de distintos tamaños. Comparar concurrencia 1, 4 y 8 conservando resultados equivalentes. Registrar tiempo total, solicitudes, conexiones nuevas, memoria y asignaciones con varias repeticiones. Separar extracción, validación y descarga.

Cierre: reutilización de conexiones cuando los cuerpos lo permiten, una solicitud por recurso y contexto deduplicado, límites respetados y ausencia de carreras. Ejecutar `go test -race ./...` en un runner compatible. Publicar mediciones reales sin prometer un multiplicador de velocidad antes de medir.

## Entrega 4: funcionalidad y descargas

Las siguientes opciones son propuestas, todavía no implementadas.

| Opción | Utilidad | Comportamiento propuesto |
| --- | --- | --- |
| `--jsonl` | Integración con otros programas. | Página de origen, URL y campos opcionales de estado HTTP, ruta local o error. Mantener texto como default. |
| `--output-dir` | Elegir destino de descargas. | Directorios por página de origen, nombres portables y sufijos estables para URLs con igual basename. |
| `--timeout` | Evitar bloqueos. | Duración validada; default inicial propuesto de 15 segundos. Implementar en entrega 2. |
| `--concurrency` | Ajustar trabajo simultáneo. | Entero positivo, default conservador y límite por host. Implementar en entrega 3. |
| `--max-body-size` | Acotar memoria y almacenamiento. | Límite para HTML y scripts, error explícito y limpieza de parciales. |
| `--insecure` | Entornos con certificados propios. | Desactivado por defecto y efecto explícito en ayuda. Implementar en entrega 2. |
| `--version` | Identificar builds. | Versión y commit suministrados durante compilación. |

Descargas: distinguir archivos externos e inline, devolver errores de escritura y cierre, preservar contenido existente y contemplar nombres reservados de Windows. Un 304 no debe crear un archivo vacío. Definir el guardado externo con `--resolve=false`, eliminando la dependencia implícita actual.

Cierre: tests con directorios temporales verifican contenido, colisiones, errores, limpieza y límites. Validar JSONL por decodificación y conservar la página de origen aunque varias páginas compartan un script.

## Entrega 5: documentación y distribución

1. Mantener el README alineado con flags, defaults, errores y ejemplos. La corrección inicial ya está preparada.
2. Agregar CI de build, tests y vet para Linux, Windows y macOS; ejecutar race en Linux.
3. Verificar instalación moderna y preparar binarios para plataformas probadas.
4. Recuperar la licencia original y sus avisos de autoría, sin inventar titulares o años.
5. Preparar changelog sobre TLS, errores visibles, nombres y deduplicación, que pueden cambiar comportamientos existentes.

Cierre: un checkout limpio compila con las instrucciones publicadas y los ejemplos coinciden con la ayuda. La publicación de una release queda como entrega posterior.

## Funciones adicionales de bajo costo

Estimaciones relativas de implementación, incluyendo tests; no son compromisos de tiempo. Dependen de completar la separación del parser y las correcciones de URLs.

| Función | Valor concreto | Esfuerzo | Alcance |
| --- | --- | --- | --- |
| Detectar `modulepreload` y `preload as=script` | Incluir referencias declaradas fuera de etiquetas script. | Bajo | Leer `link[href]`, interpretar `rel` como tokens y distinguir precarga de ejecución efectiva. Para modulepreload, filtrar destinos no JavaScript explícitos. |
| `--html-file` con `--base-url` | Analizar HTML guardado o generado en CI. | Bajo a medio | Modo offline sin solicitudes implícitas; stdin sigue siendo una lista de URLs. |
| `--same-origin` | Excluir scripts externos cuando interesan los recursos del sitio. | Bajo | Comparar esquema, hostname y puerto efectivo; aplicar antes de checks y descargas, incluyendo redirects. No confundir mismo origen con mismo dominio. |
| `--include` y `--exclude` | Seleccionar recursos por patrón y reducir ruido. | Bajo | Expresiones compiladas una vez; documentar precedencia de exclusión. |
| Metadatos de etiquetas en JSONL | Inventariar `type`, `async`, `defer`, `integrity` y `crossorigin`. | Bajo después de JSONL | Leer atributos existentes, sin ejecutar scripts ni afirmar que fueron cargados. |
| Manifiesto de descargas con SHA-256 | Comparar contenido entre ejecuciones y reconocer duplicados con distintas URLs. | Bajo a medio después de corregir descargas | Hash durante la escritura; registrar URL, origen, ruta, tamaño y hash solo tras una descarga completa. El hash no certifica confianza. |

Primera selección recomendada: precargas, mismo origen y HTML local. Después, metadatos y manifiesto sobre la salida JSONL ya prevista. Un `<script type="module" src="...">` ya entra en la extracción actual; la mejora está en referencias de precarga y en clasificar metadatos.

## Utilidad en 2026

Evaluación cualitativa, no benchmark ni medición de adopción: 5/10 para el estado funcional actual y un potencial de 8/10 como utilidad especializada después de las correcciones y funciones pequeñas.

Conserva valor para inventariar scripts declarados en HTML, archivar recursos y componer pipelines con una CLI pequeña. Los errores de URLs, HTTP y escritura limitan la confianza actual. No ejecuta JavaScript, por lo que no observa recursos que aparecen solo durante la ejecución del navegador.

[Katana](https://github.com/projectdiscovery/katana) ya ofrece crawling y un modo con navegador. Mi recomendación es mantener getJS enfocado en extracción predecible, modo offline y archivos con trazabilidad. Integrar un navegador y reconstruir dependencias JavaScript aumentaría mucho el mantenimiento; esas funciones quedan fuera de la lista de mejoras fáciles.

Sí vale la pena mantenerlo como herramienta pequeña y fiable. Priorizar las correcciones y validar que siga siendo útil en los flujos reales del usuario antes de ampliar el alcance.

## Serie de commits aprobada

El usuario corrigió la entrega: commits separados, sin PRs. Los tres PRs que ya se habían abierto se cerraron. Conservar los commits y continuar la implementación en una sola rama, `modernize/getjs-2026`. No abrir nuevos PRs ni hacer merge automático.

| Orden | Identificador de entrega | Entrega |
| --- | --- | --- |
| 1 | `modernize/01-go-modules` | Go, dependencias fijadas y plan. |
| 2 | `modernize/02-reliability` | URLs, entradas, HTTP, TLS, timeout, errores y tests. |
| 3 | `modernize/03-performance` | Conexiones, concurrencia acotada, deduplicación y benchmarks. |
| 4 | `modernize/04-downloads-jsonl` | Descargas robustas, límites, JSONL y versión. |
| 5 | `modernize/05-preloads` | Referencias de precarga. |
| 6 | `modernize/06-same-origin` | Filtro por origen y redirects. |
| 7 | `modernize/07-filters` | Inclusión y exclusión por patrón. |
| 8 | `modernize/08-offline-html` | HTML local y URL base. |
| 9 | `modernize/09-metadata` | Metadatos HTML en JSONL. |
| 10 | `modernize/10-manifest` | Manifiesto de descargas y SHA-256. |
| 11 | `modernize/11-ci-docs` | CI, documentación final y distribución. |

Cada commit incorpora sus tests y la documentación de sus opciones. Los tests de HTTP usan servidores locales. Los benchmarks deben medir resultados equivalentes. Los identificadores de entrega de la tabla son etiquetas de planificación; todos los commits nuevos se realizan en la misma rama.

## Orden recomendado

Empezar por las entregas 1 y 2: permiten medir rendimiento sin arrastrar errores de funcionamiento. Continuar con rendimiento, funcionalidad y distribución. Cada entrega debe mantener utilizables los flags existentes y explicar cualquier cambio observable.

## Referencias técnicas

- [Instalación de ejecutables en Go](https://go.dev/doc/go-get-install-deprecation).
- [Cliente HTTP y transporte de Go](https://pkg.go.dev/net/http): reutilización, timeouts y cuerpos.
- [pflag](https://github.com/spf13/pflag): sintaxis de flags largos, cortos y booleanos.
- [Versiones y política de soporte de Go](https://go.dev/doc/devel/release).
- [Precarga de módulos en HTML](https://html.spec.whatwg.org/multipage/links.html#link-type-modulepreload).
- [Katana](https://github.com/projectdiscovery/katana): capacidades de crawling y navegador utilizadas como referencia de alcance.
