import {FormEvent, useCallback, useEffect, useState} from 'react'
import {api, Config, LogEntry, Scene, Snapshot} from './api'
import './App.css'

const emptySnapshot: Snapshot = {serverRunning: false, serverAddress: '', obs: {connected: false, currentScene: ''}}

function App() {
  const [tab, setTab] = useState<'dashboard' | 'settings' | 'logs'>('dashboard')
  const [snapshot, setSnapshot] = useState<Snapshot>(emptySnapshot)
  const [config, setConfig] = useState<Config | null>(null)
  const [password, setPassword] = useState('')
  const [scenes, setScenes] = useState<Scene[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [logFilter, setLogFilter] = useState('all')
  const [busy, setBusy] = useState('')
  const [notice, setNotice] = useState<{kind: 'ok' | 'error'; text: string} | null>(null)

  const refresh = useCallback(async () => {
    try {
      const status = await api().GetSnapshot()
      setSnapshot(status)
      setScenes(status.obs.connected ? await api().GetScenes() : [])
    } catch (error) { setNotice({kind: 'error', text: String(error)}) }
  }, [])
  const refreshLogs = useCallback(async () => setLogs(await api().GetLogs(logFilter)), [logFilter])

  useEffect(() => {
    api().GetConfig().then(setConfig).catch(error => setNotice({kind: 'error', text: String(error)}))
    refresh()
    const timer = window.setInterval(refresh, 3000)
    return () => window.clearInterval(timer)
  }, [refresh])
  useEffect(() => {
    refreshLogs()
    const timer = window.setInterval(refreshLogs, 2000)
    return () => window.clearInterval(timer)
  }, [refreshLogs])

  async function action(name: string, operation: () => Promise<unknown>, success: string) {
    setBusy(name); setNotice(null)
    try {
      await operation(); setNotice({kind: 'ok', text: success}); await refresh(); await refreshLogs()
    } catch (error) { setNotice({kind: 'error', text: String(error)}) }
    finally { setBusy('') }
  }
  async function save(event: FormEvent) {
    event.preventDefault()
    if (!config) return
    await action('save', () => api().SaveConfig(config, password), 'Configurações salvas. Reinicie o servidor para aplicar a porta.')
    setPassword(''); setConfig(await api().GetConfig())
  }

  return <div className="shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark">OC</span><div><strong>OBS Control</strong><small>Local Server</small></div></div>
      <nav>
        <button className={tab === 'dashboard' ? 'active' : ''} onClick={() => setTab('dashboard')}><span>⌁</span>Painel</button>
        <button className={tab === 'settings' ? 'active' : ''} onClick={() => setTab('settings')}><span>⚙</span>Configurações</button>
        <button className={tab === 'logs' ? 'active' : ''} onClick={() => setTab('logs')}><span>≡</span>Logs</button>
      </nav>
      <div className="sidebar-status"><i className={snapshot.serverRunning ? 'dot online' : 'dot'} /><div><strong>{snapshot.serverRunning ? 'Servidor online' : 'Servidor offline'}</strong><small>{snapshot.serverAddress || 'Sem endereço ativo'}</small></div></div>
    </aside>
    <main>
      <header><div><p className="eyebrow">CONTROLE LOCAL</p><h1>{tab === 'dashboard' ? 'Painel de controle' : tab === 'settings' ? 'Configurações' : 'Logs da aplicação'}</h1></div><span className="secure">● Somente localhost</span></header>
      {notice && <div className={`notice ${notice.kind}`}>{notice.text}<button onClick={() => setNotice(null)}>×</button></div>}

      {tab === 'dashboard' && <>
        <section className="status-grid">
          <StatusCard label="Servidor local" value={snapshot.serverRunning ? 'Online' : 'Offline'} detail={snapshot.serverAddress || `127.0.0.1:${config?.server.port || 3456}`} good={snapshot.serverRunning} icon="S" />
          <StatusCard label="OBS Studio" value={snapshot.obs.connected ? 'Conectado' : 'Desconectado'} detail={config ? `${config.obsHost}:${config.obsPort}` : 'localhost:4455'} good={snapshot.obs.connected} icon="O" />
          <StatusCard label="Cena atual" value={snapshot.obs.currentScene || 'Indisponível'} detail={snapshot.obs.connected ? `${scenes.length} cenas encontradas` : 'Conecte ao OBS para consultar'} good={snapshot.obs.connected} icon="C" />
        </section>
        <section className="panel"><PanelTitle title="Ações rápidas" detail="Gerencie os serviços sem sair do painel." /><div className="actions">
          <ActionButton title="Iniciar servidor" subtitle="Ativar API REST local" icon="▶" disabled={snapshot.serverRunning || !!busy} onClick={() => action('start', api().StartServer, 'Servidor iniciado.')} />
          <ActionButton title="Parar servidor" subtitle="Desativar API REST" icon="■" danger disabled={!snapshot.serverRunning || !!busy} onClick={() => action('stop', api().StopServer, 'Servidor parado.')} />
          <ActionButton title="Reiniciar servidor" subtitle="Aplicar configurações" icon="↻" disabled={!snapshot.serverRunning || !!busy} onClick={() => action('restart', api().RestartServer, 'Servidor reiniciado.')} />
          <ActionButton title={snapshot.obs.connected ? 'Desconectar OBS' : 'Conectar ao OBS'} subtitle="Sessão WebSocket" icon="⌁" disabled={!!busy} onClick={() => action('connect', snapshot.obs.connected ? async () => api().DisconnectOBS() : api().ConnectOBS, snapshot.obs.connected ? 'OBS desconectado.' : 'OBS conectado.')} />
          <ActionButton title="Testar conexão" subtitle="Validar host e credenciais" icon="✓" disabled={!!busy} onClick={() => action('test', api().TestOBS, 'Conexão testada com sucesso.')} />
        </div></section>
        <section className="panel"><div className="panel-heading"><div><h2>Cenas do OBS</h2><p>Troque a cena ativa com um clique.</p></div><span className="counter">{scenes.length}</span></div>
          {scenes.length ? <div className="scene-list">{scenes.map(scene => <button key={scene.name} className={snapshot.obs.currentScene === scene.name ? 'selected' : ''} onClick={() => action('scene', () => api().SetScene(scene.name), `Cena alterada para “${scene.name}”.`)}><span>{scene.name.slice(0, 1).toUpperCase()}</span><strong>{scene.name}</strong>{snapshot.obs.currentScene === scene.name && <em>NO AR</em>}</button>)}</div> : <div className="empty">Conecte ao OBS para visualizar as cenas disponíveis.</div>}
        </section>
      </>}

      {tab === 'settings' && config && <form className="settings" onSubmit={save}>
        <section className="panel"><PanelTitle title="Servidor local" detail="A V1 permanece restrita ao computador local." /><div className="form-grid">
          <label>Host<input value={config.server.host} disabled /></label><label>Porta<input type="number" min="1" max="65535" value={config.server.port} onChange={e => setConfig({...config, server: {...config.server, port: Number(e.target.value)}})} /></label>
          <label className="wide">Token da API<input value={config.server.apiToken} onChange={e => setConfig({...config, server: {...config.server, apiToken: e.target.value}})} /></label>
          <Toggle label="Iniciar servidor ao abrir" checked={config.server.autoStart} onChange={value => setConfig({...config, server: {...config.server, autoStart: value}})} />
        </div></section>
        <section className="panel"><PanelTitle title="OBS Studio" detail="Dados do obs-websocket 5, integrado ao OBS atual." /><div className="form-grid">
          <label>Host<input value={config.obsHost} onChange={e => setConfig({...config, obsHost: e.target.value})} /></label><label>Porta<input type="number" min="1" max="65535" value={config.obsPort} onChange={e => setConfig({...config, obsPort: Number(e.target.value)})} /></label>
          <label className="wide">Senha<input type="password" value={password} placeholder={config.hasPassword ? 'Senha salva — deixe vazio para manter' : 'Senha configurada no OBS'} onChange={e => setPassword(e.target.value)} /></label>
          <Toggle label="Conectar ao abrir" checked={config.autoConnect} onChange={value => setConfig({...config, autoConnect: value})} /><Toggle label="Reconectar automaticamente" checked={config.autoReconnect} onChange={value => setConfig({...config, autoReconnect: value})} />
        </div></section><div className="form-actions"><button className="primary" disabled={busy === 'save'}>Salvar configurações</button></div>
      </form>}

      {tab === 'logs' && <section className="panel logs-panel"><div className="panel-heading"><div><h2>Eventos recentes</h2><p>Até 500 eventos desta sessão.</p></div><select value={logFilter} onChange={e => setLogFilter(e.target.value)}><option value="all">Todos</option><option value="server">Servidor</option><option value="obs">OBS</option><option value="requests">Requisições</option><option value="errors">Erros</option></select></div>
        <div className="log-list">{logs.length ? [...logs].reverse().map(entry => <div className={`log ${entry.level}`} key={entry.id}><time>{new Date(entry.time).toLocaleTimeString('pt-BR')}</time><span>{entry.category}</span><p>{entry.message}</p></div>) : <div className="empty">Nenhum evento para este filtro.</div>}</div>
      </section>}
    </main>
  </div>
}

function PanelTitle({title, detail}: {title: string; detail: string}) { return <div className="panel-heading"><div><h2>{title}</h2><p>{detail}</p></div></div> }
function StatusCard({label, value, detail, good, icon}: {label: string; value: string; detail: string; good: boolean; icon: string}) { return <article className="status-card"><div className="status-icon">{icon}</div><div><p>{label}</p><h3>{value}</h3><small><i className={good ? 'dot online' : 'dot'} />{detail}</small></div></article> }
function ActionButton({title, subtitle, icon, danger, disabled, onClick}: {title: string; subtitle: string; icon: string; danger?: boolean; disabled: boolean; onClick: () => void}) { return <button className={`action ${danger ? 'danger' : ''}`} disabled={disabled} onClick={onClick}><span>{icon}</span><div><strong>{title}</strong><small>{subtitle}</small></div><b>›</b></button> }
function Toggle({label, checked, onChange}: {label: string; checked: boolean; onChange: (value: boolean) => void}) { return <label className="toggle-row"><span>{label}</span><button type="button" className={checked ? 'toggle checked' : 'toggle'} onClick={() => onChange(!checked)}><i /></button></label> }
export default App
