import { Plus } from 'lucide-react'
import { AdminLibraryCreateForm } from './AdminLibraryPanelSections'
import { AdminLibraryTable } from './AdminLibraryTable'
import { LibraryStorageStats } from './LibraryStorageStats'
import { LibraryCoverDialog } from './LibraryCoverDialog'
import { LibraryPathChangeDialog } from './LibraryPathChangeDialog'
import { useAdminLibraryPanel } from './useAdminLibraryPanel'
import './AdminLibraryPanel.css'

export function AdminLibraryPanel() {
  const { libs, createForm, editableRoots, rootActions, libraryActions } = useAdminLibraryPanel()

  return (
    <div className="nook-library-manager-admin space-y-8">
      <div>
      <div className="nook-library-manager-admin-toolbar"><p>{libs.length} 个媒体库 · {libs.reduce((total, library) => total + (library.roots?.length || 1), 0)} 个来源目录</p><button type="button" className="nook-library-manager-button nook-library-manager-primary" disabled={createForm.mode !== null} onClick={createForm.openCreate}><Plus size={16} />新建媒体库</button></div>
      {createForm.mode && <AdminLibraryCreateForm key={`${createForm.mode}:${createForm.targetLibrary?.id ?? ''}`} form={createForm} />}
      <AdminLibraryTable
        libs={libs}
        disabled={createForm.creating}
        onCancelRootEdit={rootActions.cancelRootEdit}
        editableRootDraft={editableRoots.editableRootDraft}
        onEditableRootChange={editableRoots.setEditableRootDraft}
        onSaveRoot={rootActions.saveLibraryRoot}
        onScanRoot={rootActions.scanLibraryRoot}
        onToggleRoot={rootActions.toggleLibraryRoot}
        onRemoveRoot={rootActions.removeLibraryRoot}
        onScanLibrary={libraryActions.scanLibrary}
        onRemoveLibrary={libraryActions.removeLibrary}
        onAddLibraryRoot={createForm.openAppend}
        onEditLibraryCover={libraryActions.editLibraryCover}
        onToggleCarousel={libraryActions.toggleCarouselLibrary}
        onReorder={libraryActions.reorderLibraries}
      />
      </div>
      <LibraryStorageStats key={libs.map(library => library.id).sort().join(',')} />
      {rootActions.pathChange && (
        <LibraryPathChangeDialog
          {...rootActions.pathChange}
          onClose={rootActions.closePathChange}
          onSave={rootActions.confirmPathChange}
        />
      )}
      {libraryActions.coverLibrary && (
        <LibraryCoverDialog
          key={libraryActions.coverLibrary.id}
          library={libraryActions.coverLibrary}
          onClose={libraryActions.closeLibraryCover}
          onSave={libraryActions.saveLibraryCover}
        />
      )}
    </div>
  )
}
