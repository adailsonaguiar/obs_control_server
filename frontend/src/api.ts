export type ServerConfig = { host: string; port: number; autoStart: boolean; apiToken: string }
export type Config = { server: ServerConfig; obsHost: string; obsPort: number; autoConnect: boolean; autoReconnect: boolean; hasPassword: boolean }
export type Snapshot = { serverRunning: boolean; serverAddress: string; obs: { connected: boolean; currentScene: string } }
export type Scene = { name: string }
export type LogEntry = { id: number; time: string; category: string; level: string; message: string }

type Backend = {
  GetConfig(): Promise<Config>; SaveConfig(config: Config, password: string): Promise<void>; GetSnapshot(): Promise<Snapshot>
  StartServer(): Promise<void>; StopServer(): Promise<void>; RestartServer(): Promise<void>
  ConnectOBS(): Promise<void>; DisconnectOBS(): Promise<void>; TestOBS(): Promise<void>
  GetScenes(): Promise<Scene[]>; SetScene(name: string): Promise<void>; GetLogs(category: string): Promise<LogEntry[]>
}
declare global { interface Window { go: { main: { App: Backend } } } }
export const api = () => window.go.main.App
