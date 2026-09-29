export type ServerConfig = { host: string; port: number; autoStart: boolean; allowLan: boolean; apiToken: string }
export type ApplicationConfig = { launchAtLogin: boolean; minimizeToTray: boolean }
export type Config = { server: ServerConfig; application: ApplicationConfig; obsHost: string; obsPort: number; autoConnect: boolean; autoReconnect: boolean; hasPassword: boolean; activeProfile: string; profileNames: string[] }
export type Snapshot = { serverRunning: boolean; serverAddress: string; obs: { connected: boolean; currentScene: string; recording: boolean; streaming: boolean } }
export type Scene = { name: string }
export type Source = { sceneName: string; name: string; id: number; enabled: boolean }
export type LogEntry = { id: number; time: string; category: string; level: string; message: string }

type Backend = {
  GetConfig(): Promise<Config>; SaveConfig(config: Config, password: string): Promise<void>; GetSnapshot(): Promise<Snapshot>
  StartServer(): Promise<void>; StopServer(): Promise<void>; RestartServer(): Promise<void>
  ConnectOBS(): Promise<void>; DisconnectOBS(): Promise<void>; TestOBS(): Promise<void>
  GetScenes(): Promise<Scene[]>; SetScene(name: string): Promise<void>
  GetSources(sceneName: string): Promise<Source[]>; SetSourceVisible(sceneName: string, sourceName: string, visible: boolean): Promise<void>
  SetRecording(active: boolean): Promise<void>; SetStreaming(active: boolean): Promise<void>
  MovePTZ(host: string, port: number, direction: string, speed: number): Promise<void>; ZoomPTZ(host: string, port: number, direction: string, speed: number): Promise<void>
  CreateProfile(name: string): Promise<void>; SwitchProfile(name: string): Promise<void>; DeleteProfile(name: string): Promise<void>
  MinimizeToTray(): Promise<void>; GetLogs(category: string): Promise<LogEntry[]>; GetVersion(): Promise<string>
}
declare global { interface Window { go: { main: { App: Backend } } } }
export const api = () => window.go.main.App
