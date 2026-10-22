import { useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronUp, Folder, Loader2, Plus, Settings2, Trash2, X } from 'lucide-react'

import { LocalDirBrowserDialog } from '../components/LocalDirBrowserDialog'
import { LibraryScrapeSettingsFields } from './LibraryScrapeSettingsFields'
import { libraryTypeLabels, type RootDraft } from './adminLibraryPanelModel'
import type { useAdminLibraryPanel } from './useAdminLibraryPanel'

type CreateFormProps = { form: ReturnType<typeof useAdminLibraryPanel>['createForm'] }

export function AdminLibraryCreateForm({ form }: CreateFormProps) {
  const [browsingIndex, setBrowsingIndex] = useState<number | null>(null)
  const [scrapeOpen, setScrapeOpen] = useState(false)
  const formRef = useRef<HTMLFormElement>(null)
  const appending = form.mode === 'append'
  const title = appending ? `为「${form.targetLibrary?.name}」添加目录` : '新建媒体库'

  useEffect(() => {
    formRef.current?.scrollIntoView({ block: 'start' })
    formRef.current?.querySelector<HTMLInputElement>('input:not(:disabled)')?.focus({ preventScroll: true })
  }, [])

  const handleSelectDir = (path: string) => {
    if (browsingIndex === null) return
    const baseName = path.split(/[/\\]/).filter(Boolean).pop() ?? ''
    form.updateRoot(browsingIndex, { path, ...(!form.roots[browsingIndex]?.name?.trim() ? { name: baseName } : {}) })
    setBrowsingIndex(null)
  }

  return <>
    <form ref={formRef} className="nook-library-manager-form" onSubmit={form.handleCreate} aria-label={title} aria-busy={form.creating}>
      <fieldset disabled={form.creating}>
        <div className="nook-library-manager-form-heading"><h3>{title}</h3><button type="button" className="nook-library-manager-icon" aria-label="收起表单" onClick={form.close}><X size={17} /></button></div>
        {appending ? <div className="nook-library-manager-form-target"><strong>{form.targetLibrary?.name}</strong><span>{libraryTypeLabels[form.targetLibrary?.type ?? ''] ?? form.targetLibrary?.type}</span><p>已有目录：{form.targetLibrary?.roots?.map(root => root.path).join('、') || form.targetLibrary?.path}</p></div> : <div className="nook-library-manager-two-fields">
          <label>媒体库名称<input required={!form.createPerSubfolder} disabled={form.createPerSubfolder} className="input-base" value={form.name} placeholder="例如：经典电影" onChange={event => form.setName(event.target.value)} /></label>
          <label>类型<select className="input-base" value={form.type} onChange={event => form.setType(event.target.value)}>{Object.entries(libraryTypeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
        </div>}
        <div className="nook-library-manager-new-roots">{form.roots.map((root, index) => <div className="nook-library-manager-new-root" key={index}>
          <div className="nook-library-manager-path-fields">
            <label>目录名称（选填）<input className="input-base" value={root.name ?? ''} placeholder="例如：硬盘 2" onChange={event => form.updateRoot(index, { name: event.target.value })} /></label>
            <label>{form.createPerSubfolder ? '父目录' : '目录路径'}<div className="nook-library-manager-path-input"><input required className="input-base" aria-label={`新增目录路径 ${index + 1}`} value={root.path} placeholder={appending ? `/disk2/${form.targetLibrary?.name}` : '/media/经典电影'} onChange={event => form.updateRoot(index, { path: event.target.value })} /><button type="button" className="nook-library-manager-button" aria-label={`浏览目录 ${index + 1}`} onClick={() => setBrowsingIndex(index)}><Folder size={16} />浏览</button></div></label>
          </div>
          {form.roots.length > 1 && <button type="button" className="nook-library-manager-text-button nook-library-manager-danger" onClick={() => form.removeRoot(index)}><Trash2 size={14} />移除此目录</button>}
        </div>)}
          {!form.createPerSubfolder && <button type="button" className="nook-library-manager-text-button" onClick={form.addRoot}><Plus size={16} />再添加一个目录</button>}
        </div>
        {!appending && <>
          <div className="nook-library-manager-scrape-entry"><button type="button" className="nook-library-manager-button" aria-expanded={scrapeOpen} aria-controls="create-library-scrape-settings" onClick={() => setScrapeOpen(!scrapeOpen)}><Settings2 size={16} />刮削设置{scrapeOpen ? <ChevronUp size={14} /> : <ChevronDown size={14} />}</button><span>创建时一并保存</span></div>
          {form.loadingScrape && <p role="status" className="nook-library-manager-muted">正在加载刮削默认设置…</p>}
          {form.scrapeError && <p role="alert" className="nook-library-manager-danger">{form.scrapeError} <button type="button" className="nook-library-manager-text-button" onClick={() => void form.loadScrapeDefaults()}>重试</button></p>}
          {scrapeOpen && form.scrape && <section id="create-library-scrape-settings" aria-label="新媒体库刮削设置"><LibraryScrapeSettingsFields data={form.scrape} onChange={form.setScrape} /></section>}
          <details className="nook-library-manager-more-options"><summary>更多选项</summary>
            <label>封面图片链接（选填）<input type="url" className="input-base" value={form.coverURL} placeholder="粘贴封面图片直链" onChange={event => form.setCoverURL(event.target.value)} /></label>
            <label className="nook-library-manager-checkbox"><input type="checkbox" checked={form.createPerSubfolder} onChange={event => { form.setCreatePerSubfolder(event.target.checked); if (event.target.checked) form.roots.slice(1).forEach(() => form.removeRoot(1)) }} />按每个子文件夹分别创建媒体库</label>
            {form.createPerSubfolder && <p className="nook-library-manager-muted">将为第一个目录中的每个直接子文件夹创建媒体库，名称取自文件夹，并使用相同的刮削设置。</p>}
          </details>
        </>}
        {form.error && <p role="alert" className="nook-library-manager-danger">{form.error}</p>}
        <div className="nook-library-manager-form-footer"><button type="button" className="nook-library-manager-button" onClick={form.close}>取消</button><button type="submit" className="nook-library-manager-button nook-library-manager-primary" disabled={!appending && (!form.scrape || form.loadingScrape)}>{form.creating && <Loader2 size={16} className="animate-spin" />}{form.creating ? '保存中…' : appending ? '添加到此媒体库' : form.createPerSubfolder ? '按子文件夹创建媒体库' : '新建媒体库'}</button></div>
      </fieldset>
    </form>
    {browsingIndex !== null && <LocalDirBrowserDialog initialDir={form.roots[browsingIndex]?.path || undefined} title="选择媒体库目录" onSelect={handleSelectDir} onClose={() => setBrowsingIndex(null)} />}
  </>
}

export type { RootDraft }
