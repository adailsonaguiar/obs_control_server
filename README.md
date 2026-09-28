# OBS Control Server — MVP V1

Aplicação desktop local para controlar cenas do OBS Studio por meio de uma API REST protegida. O app combina um backend Go, uma interface React + TypeScript em Wails e comunicação nativa com o protocolo obs-websocket 5.

## Escopo entregue

- Aplicativo desktop Wails para macOS, Windows e Linux.
- Servidor HTTP configurável, restrito a `127.0.0.1` na V1.
- Inicialização, parada e reinicialização do servidor pela interface.
- Token Bearer gerado automaticamente para proteger comandos.
- Configuração persistente em JSON com permissão de arquivo `0600`.
- Conexão, desconexão, teste e reconexão automática com o OBS.
- Autenticação challenge/response do obs-websocket 5.
- Consulta da cena atual, listagem de cenas e troca de cena.
- Painel de status, tela de configurações e logs filtráveis.
- Buffer limitado aos 500 eventos mais recentes da sessão.

Gravação, transmissão, controle de fontes, acesso pela rede local, bandeja, macros e atalhos pertencem às versões seguintes e não fazem parte deste MVP.

## Arquitetura

```text
React + TypeScript (Wails WebView)
                │
                ▼
           App Go / Wails
         ┌──────┴──────┐
         ▼             ▼
 API REST local    Cliente WebSocket
 127.0.0.1:3456    ws://localhost:4455
         │             │
  Apps locais       OBS Studio
```

O backend está dividido em módulos pequenos:

| Diretório | Responsabilidade |
| --- | --- |
| `internal/config` | validação e persistência atômica da configuração |
| `internal/logs` | eventos em memória e filtros |
| `internal/obs` | cliente obs-websocket 5 e autenticação |
| `internal/server` | ciclo de vida e rotas da API REST |
| `frontend/src` | interface desktop e integração com os bindings Wails |

## Requisitos

- Go 1.23 ou superior.
- Node.js 20 ou superior.
- Wails CLI 2.15 ou superior.
- OBS Studio com o servidor WebSocket habilitado em **Ferramentas → Configurações do servidor WebSocket**.

## Desenvolvimento

Instale as dependências do frontend:

```bash
cd frontend
npm install
cd ..
```

Inicie o aplicativo em modo de desenvolvimento:

```bash
wails dev
```

O app cria a configuração na pasta de configurações do usuário, dentro de `obs-control-server/config.json`. A senha do OBS nunca é retornada para a interface: ela é exibida apenas como “senha salva” e um campo vazio preserva o valor atual.

## API REST

Por padrão, a API fica disponível em `http://127.0.0.1:3456`.

| Método | Rota | Autenticação | Descrição |
| --- | --- | --- | --- |
| `GET` | `/health` | não | saúde do servidor local |
| `GET` | `/obs/status` | Bearer | conexão e cena atual |
| `GET` | `/obs/scenes` | Bearer | cenas disponíveis |
| `POST` | `/obs/scene` | Bearer | troca a cena atual |
| `POST` | `/server/restart` | Bearer | reinicia a API na mesma configuração |

Exemplo:

```bash
curl -X POST http://127.0.0.1:3456/obs/scene \
  -H 'Authorization: Bearer SEU_TOKEN' \
  -H 'Content-Type: application/json' \
  -d '{"sceneName":"Camera Principal"}'
```

O token pode ser consultado e alterado na tela **Configurações** do aplicativo.

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

## Plano executado da V1

1. Base Wails com React e TypeScript.
2. Configuração persistente, defaults seguros e logs.
3. Cliente obs-websocket 5 com autenticação e comandos de cena.
4. Servidor REST local com token e ciclo start/stop/restart.
5. Integração dos serviços ao ciclo de vida do desktop.
6. Painel, configurações, cenas e visualização de logs.
7. Testes unitários, testes locais de integração e build instalável.
