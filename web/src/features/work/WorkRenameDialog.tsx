import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'

/** 修改作品名称：只改显示名，不影响作品内容与存档。 */
export function WorkRenameDialog({
  open,
  title,
  onClose,
  onSubmit,
}: {
  open: boolean
  title: string
  onClose: () => void
  onSubmit: (title: string) => void
}) {
  const { t } = useTranslation()
  const [value, setValue] = useState(title)
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) setValue(title)
        if (!o) onClose()
      }}
    >
      <DialogContent className="sm:max-w-md" onOpenAutoFocus={() => setValue(title)}>
        <DialogHeader>
          <DialogTitle>{t('library.rename')}</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            const next = value.trim()
            if (next) onSubmit(next)
          }}
        >
          <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder={t('library.renamePlaceholder')} autoFocus />
          <DialogFooter className="mt-5">
            <Button type="button" variant="ghost" onClick={onClose}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={!value.trim()}>
              {t('common.confirm')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
