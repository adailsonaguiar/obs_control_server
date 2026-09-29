import {FormEvent, useCallback, useEffect, useState} from 'react'
import {Environment} from '../wailsjs/runtime/runtime'
import {api, backendAvailable, Config, LogEntry, Scene, Snapshot, Source} from './api'
import logo from './assets/images/logo.png'
import './App.css'

const emptySnapshot: Snapshot = {serverRunning: false, serverAddress: '', obs: {connected: false, currentScene: '', recording: false, streaming: false}}

function App() {
  const [tab, setTab] = useState<'dashboard' | 'ptz' | 'settings' | 'instructions' | 'logs'>('dashboard')
  const [snapshot, setSnapshot] = useState<Snapshot>(emptySnapshot)
  const [config, setConfig] = useState<Config | null>(null)
  const [password, setPassword] = useState('')
  const [scenes, setScenes] = useState<Scene[]>([])
  const [sources, setSources] = useState<Source[]>([])
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [logFilter, setLogFilter] = useState('all')
  const [busy, setBusy] = useState('')
  const [version, setVersion] = useState('')
  const [notice, setNotice] = useState<{kind: 'ok' | 'error'; text: string} | null>(null)
  const [ptzHost, setPtzHost] = useState('192.168.1.100')
  const [ptzPort, setPtzPort] = useState(52381)
  const [ptzSpeed, setPtzSpeed] = useState(8)

  const sendPTZ = useCallback(async (kind: 'move' | 'zoom', direction: string) => {
    try {
      if (kind === 'move') await api().MovePTZ(ptzHost, ptzPort, direction, ptzSpeed)
      else await api().ZoomPTZ(ptzHost, ptzPort, direction, Math.min(ptzSpeed, 7))
    } catch (error) { setNotice({kind: 'error', text: String(error)}) }
  }, [ptzHost, ptzPort, ptzSpeed])

  useEffect(() => {
    let active = true
    try {
      Environment().then(({platform}) => {
        if (active) document.documentElement.dataset.platform = platform
      }).catch(() => undefined)
    } catch {
      // The Wails runtime is unavailable when the frontend runs in a browser.
    }
    return () => { active = false }
  }, [])

  const refresh = useCallback(async () => {
    try {
      const status = await api().GetSnapshot()
      setSnapshot(status)
      if (status.obs.connected) {
        const [nextScenes, nextSources] = await Promise.all([api().GetScenes(), api().GetSources(status.obs.currentScene)])
        setScenes(nextScenes); setSources(nextSources)
      } else {
        setScenes([]); setSources([])
      }
    } catch (error) { setNotice({kind: 'error', text: String(error)}) }
  }, [])
  const refreshLogs = useCallback(async () => {
    try { setLogs(await api().GetLogs(logFilter)) }
    catch (error) { if (backendAvailable()) setNotice({kind: 'error', text: String(error)}) }
  }, [logFilter])

  useEffect(() => {
    if (!backendAvailable()) {
      setNotice({kind: 'error', text: 'Backend nativo indisponível. Abra esta tela pelo aplicativo OBS Remote Deck ou use “wails dev”.'})
      return
    }
    api().GetConfig().then(setConfig).catch(error => setNotice({kind: 'error', text: String(error)}))
    api().GetVersion().then(setVersion).catch(() => undefined)
    refresh()
    const timer = window.setInterval(refresh, 3000)
    return () => window.clearInterval(timer)
  }, [refresh])
  useEffect(() => {
    if (!backendAvailable()) return
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
  async function createProfile() {
    const name = window.prompt('Nome do novo perfil:')?.trim()
    if (!name) return
    await action('profile', () => api().CreateProfile(name), `Perfil “${name}” criado.`)
    setConfig(await api().GetConfig())
  }
  async function switchProfile(name: string) {
    await action('profile', () => api().SwitchProfile(name), `Perfil “${name}” ativado.`)
    setPassword(''); setConfig(await api().GetConfig())
  }
  async function deleteProfile() {
    if (!config || !window.confirm(`Excluir o perfil “${config.activeProfile}”?`)) return
    await action('profile', () => api().DeleteProfile(config.activeProfile), 'Perfil excluído.')
    setConfig(await api().GetConfig())
  }

  return <><div className="window-titlebar" aria-hidden="true" /><div className="shell">
    <aside className="sidebar">
      <div className="brand"><span className="brand-mark"><img src={logo} alt="" /></span><div><strong>OBS Remote Deck</strong><small>Local Server</small></div></div>
      <nav>
        <button className={tab === 'dashboard' ? 'active' : ''} onClick={() => setTab('dashboard')}><span>⌁</span>Painel</button>
        <button className={tab === 'ptz' ? 'active' : ''} onClick={() => setTab('ptz')}><span>✥</span>Câmera PTZ</button>
        <button className={tab === 'settings' ? 'active' : ''} onClick={() => setTab('settings')}><span>⚙</span>Configurações</button>
        <button className={tab === 'instructions' ? 'active' : ''} onClick={() => setTab('instructions')}><span>?</span>Como conectar</button>
        <button className={tab === 'logs' ? 'active' : ''} onClick={() => setTab('logs')}><span>≡</span>Logs</button>
      </nav>
      <div className="sidebar-status"><i className={snapshot.serverRunning ? 'dot online' : 'dot'} /><div><strong>{snapshot.serverRunning ? 'Servidor online' : 'Servidor offline'}</strong><small>{snapshot.serverAddress || 'Sem endereço ativo'}</small></div></div>
      <div className="app-version">OBS Remote Deck{version && <span>v{version}</span>}</div>
    </aside>
    <main>
      <header><div><p className="eyebrow">CONTROLE LOCAL</p><h1>{tab === 'dashboard' ? 'Painel de controle' : tab === 'ptz' ? 'Controle de câmera PTZ' : tab === 'settings' ? 'Configurações' : tab === 'instructions' ? 'Como conectar ao OBS' : 'Logs da aplicação'}</h1></div><span className="secure">● Somente localhost</span></header>
      {notice && <div className={`notice ${notice.kind}`}>{notice.text}<button onClick={() => setNotice(null)}>×</button></div>}

      {tab === 'dashboard' && <>
        <section className="status-grid">
          <StatusCard label="Servidor local" value={snapshot.serverRunning ? 'Online' : 'Offline'} detail={snapshot.serverAddress || `127.0.0.1:${config?.server.port || 3456}`} good={snapshot.serverRunning} icon="S" />
          <StatusCard label="OBS Studio" value={snapshot.obs.connected ? 'Conectado' : 'Desconectado'} detail={config ? `${config.obsHost}:${config.obsPort}` : 'localhost:4455'} good={snapshot.obs.connected} icon="O" />
          <StatusCard label="Cena atual" value={snapshot.obs.currentScene || 'Indisponível'} detail={snapshot.obs.connected ? `${scenes.length} cenas encontradas` : 'Conecte ao OBS para consultar'} good={snapshot.obs.connected} icon="C" />
          <StatusCard label="Gravação" value={snapshot.obs.recording ? 'Ativa' : 'Inativa'} detail={snapshot.obs.recording ? 'Capturando no OBS' : 'Aguardando início'} good={snapshot.obs.recording} icon="R" />
          <StatusCard label="Transmissão" value={snapshot.obs.streaming ? 'Ao vivo' : 'Inativa'} detail={snapshot.obs.streaming ? 'Saída de stream ativa' : 'Aguardando início'} good={snapshot.obs.streaming} icon="T" />
        </section>
        <section className="panel"><PanelTitle title="Ações rápidas" detail="Gerencie os serviços sem sair do painel." /><div className="actions">
          <ActionButton title="Iniciar servidor" subtitle="Ativar API REST local" icon="▶" disabled={snapshot.serverRunning || !!busy} onClick={() => action('start', api().StartServer, 'Servidor iniciado.')} />
          <ActionButton title="Parar servidor" subtitle="Desativar API REST" icon="■" danger disabled={!snapshot.serverRunning || !!busy} onClick={() => action('stop', api().StopServer, 'Servidor parado.')} />
          <ActionButton title="Reiniciar servidor" subtitle="Aplicar configurações" icon="↻" disabled={!snapshot.serverRunning || !!busy} onClick={() => action('restart', api().RestartServer, 'Servidor reiniciado.')} />
          <ActionButton title={snapshot.obs.connected ? 'Desconectar OBS' : 'Conectar ao OBS'} subtitle="Sessão WebSocket" icon="⌁" disabled={!!busy} onClick={() => action('connect', snapshot.obs.connected ? async () => api().DisconnectOBS() : api().ConnectOBS, snapshot.obs.connected ? 'OBS desconectado.' : 'OBS conectado.')} />
          <ActionButton title="Testar conexão" subtitle="Validar host e credenciais" icon="✓" disabled={!!busy} onClick={() => action('test', api().TestOBS, 'Conexão testada com sucesso.')} />
          <ActionButton title={snapshot.obs.recording ? 'Parar gravação' : 'Iniciar gravação'} subtitle="Controle da saída local" icon={snapshot.obs.recording ? '■' : '●'} danger={snapshot.obs.recording} disabled={!snapshot.obs.connected || !!busy} onClick={() => action('recording', () => api().SetRecording(!snapshot.obs.recording), snapshot.obs.recording ? 'Gravação parada.' : 'Gravação iniciada.')} />
          <ActionButton title={snapshot.obs.streaming ? 'Parar transmissão' : 'Iniciar transmissão'} subtitle="Controle da saída ao vivo" icon={snapshot.obs.streaming ? '■' : '◉'} danger={snapshot.obs.streaming} disabled={!snapshot.obs.connected || !!busy} onClick={() => action('streaming', () => api().SetStreaming(!snapshot.obs.streaming), snapshot.obs.streaming ? 'Transmissão parada.' : 'Transmissão iniciada.')} />
          <ActionButton title="Minimizar para bandeja" subtitle="Continuar em segundo plano" icon="—" disabled={!!busy} onClick={() => api().MinimizeToTray()} />
        </div></section>
        <section className="panel"><div className="panel-heading"><div><h2>Cenas do OBS</h2><p>Troque a cena ativa com um clique.</p></div><span className="counter">{scenes.length}</span></div>
          {scenes.length ? <div className="scene-list">{scenes.map(scene => <button key={scene.name} className={snapshot.obs.currentScene === scene.name ? 'selected' : ''} onClick={() => action('scene', () => api().SetScene(scene.name), `Cena alterada para “${scene.name}”.`)}><span>{scene.name.slice(0, 1).toUpperCase()}</span><strong>{scene.name}</strong>{snapshot.obs.currentScene === scene.name && <em>NO AR</em>}</button>)}</div> : <div className="empty">Conecte ao OBS para visualizar as cenas disponíveis.</div>}
        </section>
        <section className="panel"><div className="panel-heading"><div><h2>Fontes da cena</h2><p>Mostre ou oculte itens de “{snapshot.obs.currentScene || 'nenhuma cena'}”.</p></div><span className="counter">{sources.length}</span></div>
          {sources.length ? <div className="source-list">{sources.map(source => <div className="source-row" key={source.id}><span className={source.enabled ? 'source-state visible' : 'source-state'}>{source.enabled ? 'VISÍVEL' : 'OCULTA'}</span><strong>{source.name}</strong><button disabled={!!busy} onClick={() => action('source', () => api().SetSourceVisible(source.sceneName, source.name, !source.enabled), `Fonte “${source.name}” ${source.enabled ? 'ocultada' : 'exibida'}.`)}>{source.enabled ? 'Ocultar' : 'Mostrar'}</button></div>)}</div> : <div className="empty">Nenhuma fonte disponível na cena atual.</div>}
        </section>
      </>}

      {tab === 'ptz' && <section className="panel ptz-panel">
        <PanelTitle title="VISCA over IP" detail="Segure um botão para mover a câmera; ao soltar, o movimento para." />
        <div className="ptz-settings">
          <label>IP da câmera<input value={ptzHost} inputMode="decimal" onChange={event => setPtzHost(event.target.value)} /></label>
          <label>Porta UDP<input type="number" min="1" max="65535" value={ptzPort} onChange={event => setPtzPort(Number(event.target.value))} /></label>
          <label>Velocidade ({ptzSpeed})<input type="range" min="1" max="24" value={ptzSpeed} onChange={event => setPtzSpeed(Number(event.target.value))} /></label>
        </div>
        <div className="ptz-controls">
          <div><h3>Movimento</h3><div className="ptz-pad">
            <span /><PTZButton label="↑" start={() => sendPTZ('move', 'up')} stop={() => sendPTZ('move', 'stop')} /><span />
            <PTZButton label="←" start={() => sendPTZ('move', 'left')} stop={() => sendPTZ('move', 'stop')} /><button onClick={() => sendPTZ('move', 'stop')} aria-label="Parar movimento">■</button><PTZButton label="→" start={() => sendPTZ('move', 'right')} stop={() => sendPTZ('move', 'stop')} />
            <span /><PTZButton label="↓" start={() => sendPTZ('move', 'down')} stop={() => sendPTZ('move', 'stop')} /><span />
          </div></div>
          <div><h3>Zoom</h3><div className="ptz-zoom"><PTZButton label="＋ Aproximar" start={() => sendPTZ('zoom', 'in')} stop={() => sendPTZ('zoom', 'stop')} /><PTZButton label="− Afastar" start={() => sendPTZ('zoom', 'out')} stop={() => sendPTZ('zoom', 'stop')} /></div></div>
        </div>
        <p className="ptz-note">A câmera deve estar na mesma rede e com VISCA over IP habilitado. Porta padrão: 52381/UDP.</p>
      </section>}

      {tab === 'settings' && config && <form className="settings" onSubmit={save}>
        <section className="panel"><div className="panel-heading"><div><h2>Perfil de ambiente</h2><p>Separe configurações para estúdio, produção ou testes.</p></div></div><div className="profile-toolbar">
          <select value={config.activeProfile} onChange={e => switchProfile(e.target.value)}>{config.profileNames.map(name => <option key={name} value={name}>{name}</option>)}</select>
          <button type="button" onClick={createProfile}>Novo perfil</button><button type="button" className="danger-button" disabled={config.profileNames.length <= 1} onClick={deleteProfile}>Excluir</button>
        </div></section>
        <section className="panel"><PanelTitle title="Servidor local" detail="Escolha entre acesso somente local ou pela rede LAN." /><div className="form-grid">
          <label>Host<input value={config.server.host} disabled /></label><label>Porta<input type="number" min="1" max="65535" value={config.server.port} onChange={e => setConfig({...config, server: {...config.server, port: Number(e.target.value)}})} /></label>
          <label className="wide">Token da API<input value={config.server.apiToken} onChange={e => setConfig({...config, server: {...config.server, apiToken: e.target.value}})} /></label>
          <Toggle label="Iniciar servidor ao abrir" checked={config.server.autoStart} onChange={value => setConfig({...config, server: {...config.server, autoStart: value}})} />
          <Toggle label="Permitir acesso pela rede local" checked={config.server.allowLan} onChange={value => setConfig({...config, server: {...config.server, allowLan: value, host: value ? '0.0.0.0' : '127.0.0.1'}})} />
          {config.server.allowLan && <p className="lan-warning">O servidor ficará acessível na rede. Mantenha um token forte e reinicie o servidor após salvar.</p>}
        </div></section>
        <section className="panel"><PanelTitle title="OBS Studio" detail="Dados do obs-websocket 5, integrado ao OBS atual." /><div className="form-grid">
          <label>Host<input value={config.obsHost} onChange={e => setConfig({...config, obsHost: e.target.value})} /></label><label>Porta<input type="number" min="1" max="65535" value={config.obsPort} onChange={e => setConfig({...config, obsPort: Number(e.target.value)})} /></label>
          <label className="wide">Senha<input type="password" value={password} placeholder={config.hasPassword ? 'Senha salva — deixe vazio para manter' : 'Senha configurada no OBS'} onChange={e => setPassword(e.target.value)} /></label>
          <Toggle label="Conectar ao abrir" checked={config.autoConnect} onChange={value => setConfig({...config, autoConnect: value})} /><Toggle label="Reconectar automaticamente" checked={config.autoReconnect} onChange={value => setConfig({...config, autoReconnect: value})} />
        </div></section>
        <section className="panel"><PanelTitle title="Aplicativo" detail="Comportamento do OBS Remote Deck no sistema operacional." /><div className="form-grid">
          <Toggle label="Iniciar com o sistema" checked={config.application.launchAtLogin} onChange={value => setConfig({...config, application: {...config.application, launchAtLogin: value}})} />
          <Toggle label="Continuar em segundo plano ao fechar" checked={config.application.minimizeToTray} onChange={value => setConfig({...config, application: {...config.application, minimizeToTray: value}})} />
        </div></section><div className="form-actions"><button className="primary" disabled={busy === 'save'}>Salvar configurações</button></div>
      </form>}

      {tab === 'instructions' && <div className="instructions">
        <section className="instructions-intro">
          <div><p className="eyebrow">OBS WEBSOCKET 5</p><h2>Conecte o OBS em poucos passos</h2><p>O OBS Studio 28 ou mais recente já inclui o servidor WebSocket necessário. Mantenha o OBS aberto durante a configuração.</p></div>
          <span className="version-badge">OBS 28+</span>
        </section>

        <ol className="instruction-steps">
          <li><span className="step-number">1</span><div><h3>Abra as configurações do WebSocket</h3><p>No OBS Studio, acesse <strong>Ferramentas</strong> → <strong>Configurações do servidor WebSocket</strong>.</p><p className="platform-note">No macOS, o menu “Ferramentas” fica na barra de menus no topo da tela.</p></div></li>
          <li><span className="step-number">2</span><div><h3>Ative o servidor</h3><p>Marque <strong>Ativar servidor WebSocket</strong>. Mantenha a porta padrão <code>4455</code>, salvo se ela já estiver sendo usada.</p></div></li>
          <li><span className="step-number">3</span><div><h3>Copie a senha</h3><p>Mantenha <strong>Ativar autenticação</strong> selecionado. Clique em <strong>Mostrar informações de conexão</strong> para consultar e copiar a senha, ou defina uma nova.</p><div className="security-tip"><strong>Importante:</strong> não compartilhe essa senha; ela permite controlar cenas, gravações e transmissões do OBS.</div></div></li>
          <li><span className="step-number">4</span><div><h3>Preencha os dados nesta aplicação</h3><div className="connection-values"><div><small>HOST</small><code>localhost</code></div><div><small>PORTA</small><code>4455</code></div><div><small>SENHA</small><code>A senha exibida no OBS</code></div></div><p>Se o OBS estiver em outro computador, use o endereço IP dele no lugar de <code>localhost</code> e permita a porta no firewall.</p></div></li>
          <li><span className="step-number">5</span><div><h3>Salve e teste</h3><p>Abra as configurações, informe os dados e salve. Depois volte ao Painel e selecione <strong>Testar conexão</strong> ou <strong>Conectar ao OBS</strong>.</p><button className="primary instruction-action" type="button" onClick={() => setTab('settings')}>Abrir configurações</button></div></li>
        </ol>

        <section className="troubleshooting panel">
          <PanelTitle title="Se a conexão não funcionar" detail="Confira estes pontos antes de tentar novamente." />
          <div className="troubleshooting-grid">
            <div><strong>Conexão recusada</strong><p>Confirme que o OBS está aberto, o servidor WebSocket está ativado e a porta é a mesma nos dois aplicativos.</p></div>
            <div><strong>Falha de autenticação</strong><p>Copie novamente a senha do OBS. Ela diferencia maiúsculas, minúsculas e caracteres especiais.</p></div>
            <div><strong>OBS em outro computador</strong><p>Use o IP local do computador do OBS, mantenha ambos na mesma rede e libere a porta no firewall.</p></div>
            <div><strong>OBS anterior à versão 28</strong><p>Atualize o OBS ou instale manualmente uma versão compatível do plugin obs-websocket 5.</p></div>
          </div>
        </section>
      </div>}

      {tab === 'logs' && <section className="panel logs-panel"><div className="panel-heading"><div><h2>Eventos recentes</h2><p>Até 500 eventos desta sessão.</p></div><select value={logFilter} onChange={e => setLogFilter(e.target.value)}><option value="all">Todos</option><option value="server">Servidor</option><option value="obs">OBS</option><option value="ptz">PTZ</option><option value="requests">Requisições</option><option value="errors">Erros</option></select></div>
        <div className="log-list">{logs.length ? [...logs].reverse().map(entry => <div className={`log ${entry.level}`} key={entry.id}><time>{new Date(entry.time).toLocaleTimeString('pt-BR')}</time><span>{entry.category}</span><p>{entry.message}</p></div>) : <div className="empty">Nenhum evento para este filtro.</div>}</div>
      </section>}
    </main>
  </div></>
}

function PanelTitle({title, detail}: {title: string; detail: string}) { return <div className="panel-heading"><div><h2>{title}</h2><p>{detail}</p></div></div> }
function StatusCard({label, value, detail, good, icon}: {label: string; value: string; detail: string; good: boolean; icon: string}) { return <article className="status-card"><div className="status-icon">{icon}</div><div><p>{label}</p><h3>{value}</h3><small><i className={good ? 'dot online' : 'dot'} />{detail}</small></div></article> }
function ActionButton({title, subtitle, icon, danger, disabled, onClick}: {title: string; subtitle: string; icon: string; danger?: boolean; disabled: boolean; onClick: () => void}) { return <button className={`action ${danger ? 'danger' : ''}`} disabled={disabled} onClick={onClick}><span>{icon}</span><div><strong>{title}</strong><small>{subtitle}</small></div><b>›</b></button> }
function PTZButton({label, start, stop}: {label: string; start: () => void; stop: () => void}) { return <button onPointerDown={event => { event.currentTarget.setPointerCapture(event.pointerId); start() }} onPointerUp={stop} onPointerCancel={stop}>{label}</button> }
function Toggle({label, checked, onChange}: {label: string; checked: boolean; onChange: (value: boolean) => void}) { return <label className="toggle-row"><span>{label}</span><button type="button" className={checked ? 'toggle checked' : 'toggle'} onClick={() => onChange(!checked)}><i /></button></label> }
export default App
