import { useEffect, useState } from 'react'
import { ExternalLink, Github } from 'lucide-react'

import { systemAPI } from '../api/system'
import { formatAppVersion } from '../utils/formatAppVersion'
import { DatabaseSettingsPanel } from './DatabaseSettingsPanel'
import { VersionUpdatePanel } from './VersionUpdatePanel'
import { DiagnosticsPanel } from './DiagnosticsPanel'
import './AboutSettingsPanel.css'

type SystemInfo = Awaited<ReturnType<typeof systemAPI.info>>
const GITHUB_REPO_URL = 'https://github.com/Ferrymania/nook'

export function AboutSettingsPanel() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let disposed = false
    systemAPI.info()
      .then((value) => { if (!disposed) setInfo(value) })
      .catch(() => undefined)
      .finally(() => { if (!disposed) setLoading(false) })
    return () => { disposed = true }
  }, [])

  return (
    <div className="about-settings">
      <header className="about-identity">
        <div className="about-product">
          <img src="/brand/logo-64.png" alt="" width={48} height={48} />
          <div className="about-product-name">
            <h2>{info?.name || 'Nook'}</h2>
            <span className="about-version">
              {loading && !info ? '读取版本…' : formatAppVersion(info?.version)}
            </span>
          </div>
        </div>
        <a href={GITHUB_REPO_URL} target="_blank" rel="noreferrer" className="about-button">
          <Github size={16} aria-hidden="true" />GitHub<ExternalLink size={14} aria-hidden="true" />
        </a>
      </header>

      <VersionUpdatePanel />
      <DatabaseSettingsPanel />
      <DiagnosticsPanel />
    </div>
  )
}
