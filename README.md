# OBS Remote Deck Server — Versão 2

Aplicação desktop local para controlar cenas do OBS Studio por meio de uma API REST protegida. O app combina um backend Go, uma interface React + TypeScript em Wails e comunicação nativa com o protocolo obs-websocket 5.

## Escopo entregue

- Aplicativo desktop Wails para macOS, Windows e Linux.
- Servidor HTTP configurável, local por padrão e opcionalmente disponível na LAN.
- Inicialização, parada e reinicialização do servidor pela interface.
- Token Bearer gerado automaticamente para proteger comandos.
- Configuração persistente em JSON com permissão de arquivo `0600`.
- Conexão, desconexão, teste e reconexão automática com o OBS.
- Autenticação challenge/response do obs-websocket 5.
- Consulta da cena atual, listagem de cenas e troca de cena.
- Controle de gravação e transmissão.
- Listagem de fontes e controle de visibilidade por cena.
- WebSocket autenticado para eventos em tempo real.
- Perfis independentes de ambiente.
- Inicialização automática com macOS, Windows e Linux.
- Execução em segundo plano ao fechar a janela.
- Painel de status, tela de configurações e logs filtráveis.
- Buffer limitado aos 500 eventos mais recentes da sessão.
- Controle básico de câmeras PTZ por VISCA over IP, com direção e zoom.

Macros, atalhos globais, Stream Deck e permissões granulares permanecem reservados para a versão 3.

## Arquitetura

```text
React + TypeScript (Wails WebView)
                │
                ▼
           App Go / Wails
         ┌──────┴──────┐
         ▼             ▼
 API REST + WS     Cliente WebSocket
 :3456/events      ws://localhost:4455
         │             │
  Apps locais       OBS Studio
```

O backend está dividido em módulos pequenos:

| Diretório            | Responsabilidade                                     |
| -------------------- | ---------------------------------------------------- |
| `internal/config`    | validação e persistência atômica da configuração     |
| `internal/autostart` | inicialização automática multiplataforma             |
| `internal/events`    | distribuição de eventos em tempo real                |
| `internal/logs`      | eventos em memória e filtros                         |
| `internal/obs`       | cliente obs-websocket 5 e autenticação               |
| `internal/server`    | ciclo de vida e rotas da API REST                    |
| `frontend/src`       | interface desktop e integração com os bindings Wails |

## Requisitos

- Go 1.23 ou superior.
- Node.js 20 ou superior.
- Wails CLI 2.15 ou superior.
- OBS Studio com o servidor WebSocket habilitado em **Ferramentas → Configurações do servidor WebSocket**.

## Executar localmente

### 1. Preparar o ambiente

Entre na pasta do projeto e confirme que Go, Node.js, npm e Wails estão disponíveis:

```bash
cd obs_control_server
go version
node --version
npm --version
wails version
```

Caso ainda não tenha o Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Use o diagnóstico do Wails para identificar dependências nativas ausentes na plataforma:

```bash
wails doctor
```

### 2. Instalar as dependências

```bash
go mod download
npm --prefix frontend install
```

### 3. Configurar o OBS Studio

1. Abra o OBS Studio.
2. Acesse **Ferramentas → Configurações do servidor WebSocket**.
3. Ative o servidor WebSocket.
4. Mantenha a porta `4455` ou anote a porta escolhida.
5. Defina uma senha e salve.

### 4. Executar em desenvolvimento

Com o OBS aberto, execute na raiz de `obs_control_server`:

```bash
wails dev
```

Na interface do aplicativo, abra **Configurações** e informe:

- Host do OBS: `localhost`.
- Porta do OBS: `4455`.
- Senha: a senha definida no OBS Studio.

Salve, retorne ao painel e use **Testar conexão**. O servidor HTTP inicia por padrão em `http://127.0.0.1:3456`.

### 5. Validar a API local

O endpoint de saúde não exige autenticação:

```bash
curl http://127.0.0.1:3456/health
```

Copie o token exibido em **Configurações → Token da API** para testar uma rota protegida:

```bash
curl http://127.0.0.1:3456/obs/status \
  -H 'Authorization: Bearer SEU_TOKEN'
```

### 6. Gerar e executar o bundle

```bash
wails build
```

O resultado é criado em `build/bin`. No macOS, por exemplo:

```bash
open "build/bin/OBS Remote Deck.app"
```

No Linux ou Windows, execute o binário correspondente gerado dentro de `build/bin`.

## Gerar os instaladores

Execute os comandos abaixo na raiz de `obs_control_server`.

### macOS — DMG

Gera a aplicação para Macs Apple Silicon e cria um DMG com atalho para a pasta Aplicativos. A pasta `build/dmg` é somente uma área temporária; o instalador final será criado em `dist`.

```bash
wails build -platform darwin/arm64 -o "OBS Remote Deck"

mkdir -p build/dmg dist
cp -R "build/bin/OBS Remote Deck.app" build/dmg/
ln -sfn /Applications build/dmg/Applications

hdiutil create \
  -volname "OBS Remote Deck" \
  -srcfolder build/dmg \
  -ov -format UDZO \
  dist/OBS-Remote-Deck.dmg

ls -lh dist/OBS-Remote-Deck.dmg
```

Resultado:

```text
dist/OBS-Remote-Deck-2.0.0-macOS-arm64.dmg
```

É normal que `build/dmg` contenha somente `OBS Remote Deck.app` e o atalho `Applications`: esse é exatamente o conteúdo colocado dentro do DMG pelo comando `hdiutil create`.

### Windows — instalador EXE

No macOS, instale uma única vez as ferramentas de compilação:

```bash
brew install mingw-w64 nsis
```

Depois gere o executável e o instalador:

```bash
CC=x86_64-w64-mingw32-gcc \
CXX=x86_64-w64-mingw32-g++ \
CGO_ENABLED=1 \
wails build \
  -platform windows/amd64 \
  -nsis \
  -o "OBS Remote Deck.exe"
```

Resultados:

```text
build/bin/OBS Remote Deck.exe
build/bin/OBS Remote Deck-amd64-installer.exe
```

O primeiro arquivo é a aplicação portátil. O segundo é o instalador com assistente, atalhos e desinstalador.

### Arquivos locais

O aplicativo salva a configuração no diretório de configurações do usuário:

- macOS: `~/Library/Application Support/obs-control-server/config.json`.
- Linux: `~/.config/obs-control-server/config.json`.
- Windows: `%AppData%\obs-control-server\config.json`.

A senha do OBS nunca é retornada para a interface: ela aparece apenas como “senha salva”, e deixar o campo vazio preserva o valor atual. O arquivo de configuração é criado com permissão restrita ao usuário quando o sistema operacional oferece esse controle.

## API REST

Por padrão, a API fica disponível em `http://127.0.0.1:3456`.

| Método | Rota                                      | Autenticação    | Descrição                            |
| ------ | ----------------------------------------- | --------------- | ------------------------------------ |
| `GET`  | `/health`                                 | não             | saúde do servidor local              |
| `GET`  | `/obs/status`                             | Bearer          | conexão e cena atual                 |
| `GET`  | `/obs/scenes`                             | Bearer          | cenas disponíveis                    |
| `POST` | `/obs/scene`                              | Bearer          | troca a cena atual                   |
| `GET`  | `/obs/preview?sceneName=&width=&quality=` | Bearer          | imagem JPEG otimizada da cena no ar  |
| `GET`  | `/obs/sources?sceneName=`                 | Bearer          | fontes de uma cena                   |
| `POST` | `/obs/source/show`                        | Bearer          | exibe uma fonte                      |
| `POST` | `/obs/source/hide`                        | Bearer          | oculta uma fonte                     |
| `POST` | `/obs/recording/start`                    | Bearer          | inicia a gravação                    |
| `POST` | `/obs/recording/stop`                     | Bearer          | para a gravação                      |
| `POST` | `/obs/stream/start`                       | Bearer          | inicia a transmissão                 |
| `POST` | `/obs/stream/stop`                        | Bearer          | para a transmissão                   |
| `POST` | `/ptz/move`                              | Bearer          | move ou para uma câmera VISCA IP     |
| `POST` | `/ptz/zoom`                              | Bearer          | controla ou para o zoom VISCA IP     |
| `GET`  | `/events?token=`                          | query ou Bearer | WebSocket de eventos                 |
| `POST` | `/server/restart`                         | Bearer          | reinicia a API na mesma configuração |

Exemplo:

```bash
curl -X POST http://127.0.0.1:3456/obs/scene \
  -H 'Authorization: Bearer SEU_TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{"sceneName":"Camera Principal"}'
```

O token pode ser consultado e alterado na tela **Configurações** do aplicativo.

Para acessar de outro dispositivo, ative **Permitir acesso pela rede local**, salve e reinicie o servidor. Use o IP local do computador no lugar de `127.0.0.1` e mantenha o token em todas as chamadas. CORS está habilitado para clientes web; a autenticação continua obrigatória.

Exemplo de conexão aos eventos:

```js
const events = new WebSocket(
  "ws://IP-DO-COMPUTADOR:3456/events?token=SEU_TOKEN",
);
events.onmessage = ({ data }) => console.log(JSON.parse(data));
```

## Testes e build

Execute a suíte Go com detector de corridas:

```bash
go test ./... -race
```

Valide o frontend:

```bash
npm --prefix frontend run build
```

Gere o aplicativo instalável da plataforma atual:

```bash
wails build
```

Os testes usam servidores HTTP e WebSocket locais em portas efêmeras para validar autenticação, proteção da API e o fluxo completo de cenas sem exigir um OBS aberto.

## Plano executado

1. Base Wails com React e TypeScript.
2. Configuração persistente, defaults seguros e logs.
3. Cliente obs-websocket 5 com autenticação e comandos de cena.
4. Servidor REST local com token e ciclo start/stop/restart.
5. Integração dos serviços ao ciclo de vida do desktop.
6. Painel, configurações, cenas e visualização de logs.
7. Testes unitários, testes locais de integração e build instalável.
8. Controle de gravação, transmissão e fontes.
9. API WebSocket para eventos OBS e comandos externos.
10. Perfis de ambiente e acesso LAN protegido.
11. Inicialização com o sistema e execução em segundo plano.
