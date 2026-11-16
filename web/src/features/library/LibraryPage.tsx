import { useQueryClient } from '@tanstack/react-query'
import { PackageOpen, Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { toast } from 'sonner'

import { errorMessage } from '@/api/errors'
import { qk, useRecentSaves, useRenameWork, useVersionActions, useWorks } from '@/api/queries'
import type { Save, WorkSummary } from '@/api/types'
import { confirmAction } from '@/components/common/confirm'
import { LoadError } from '@/components/common/LoadError'
import { PageHeader, SectionHeading } from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelative, saveTitle } from '@/lib/format'

import { WorkRenameDialog } from '@/features/work/WorkRenameDialog'

import { pickedFromDrop } from './dropImport'
import { ImportDialog } from './ImportDialog'
import { useFileDrop } from './useFileDrop'
import { useWorkImport } from './useWorkImport'
import { WorkCover } from './WorkCover'

/** 作品网格：列宽固定（auto-fill），作品多少都不改变封面大小。 */
const SHELF_GRID =
  'grid grid-cols-[repeat(auto-fill,minmax(136px,1fr))] gap-x-8 gap-y-12 sm:grid-cols-[repeat(auto-fill,minmax(152px,1fr))]'

/**
 * "继续阅读"只占一行：每种宽度下只显示刚好排满一行的张数，不留半行。
 *   窄屏 1 列（3 张，竖排）· sm 2 列 · xl 及更宽 3 列（再多列存档名就要被截断了）
 */
function slipVisibility(i: number) {
  if (i < 2) return undefined
  if (i === 2) return 'sm:max-xl:hidden'
  return 'hidden'
}

/** 书架（首页）：继续阅读 + 全部作品。把 zip 包或作品文件夹拖到页面任意位置即可导入。 */
export function LibraryPage() {
  const { t } = useTranslation()
  const works = useWorks()
  const recent = useRecentSaves()
  const [importing, setImporting] = useState(false)
  const imp = useWorkImport()
  const dragging = useFileDrop((dt) => {
    if (imp.busy) {
      toast(t('import.busy'))
      return
    }
    imp.startFrom(pickedFromDrop(dt)) // 必须在 drop 事件中同步取出条目
    setImporting(true)
  })

  return (
    <>
      <PageHeader
        title={t('library.shelf')}
        actions={
          <Button variant="outline" size="sm" onClick={() => setImporting(true)}>
            <Plus />
            {t('library.import')}
          </Button>
        }
      />

      {recent.data && recent.data.length > 0 && (
        <section aria-labelledby="continue-heading" className="mb-14">
          <SectionHeading id="continue-heading">{t('library.continueReading')}</SectionHeading>
          <ul className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {recent.data.map((s, i) => (
              <li key={s.id} className={slipVisibility(i)}>
                <SaveSlip save={s} />
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="shelf-heading">
        <SectionHeading id="shelf-heading" count={works.data?.length}>
          {t('library.all')}
        </SectionHeading>

        {works.isPending && <ShelfSkeleton />}
        {works.isError && <LoadError error={works.error} onRetry={() => void works.refetch()} />}
        {works.data?.length === 0 && <EmptyShelf onImport={() => setImporting(true)} />}
        {works.data && works.data.length > 0 && (
          <ul className={SHELF_GRID}>
            {works.data.map((w) => (
              <li key={w.id}>
                <ShelfBook work={w} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <ImportDialog open={importing} onOpenChange={setImporting} imp={imp} />
      {dragging && <DropOverlay />}
    </>
  )
}

/** 拖着文件经过窗口时的遮罩：整页都是放置区。 */
function DropOverlay() {
  const { t } = useTranslation()
  return (
    <div aria-hidden className="pointer-events-none fixed inset-0 z-[60] bg-paper/80 p-4 backdrop-blur-[2px] sm:p-6">
      <div className="flex size-full flex-col items-center justify-center rounded-2xl border-2 border-dashed border-gold/70 text-center">
        <PackageOpen className="size-8 text-gold" strokeWidth={1.5} />
        <p className="mt-4 title-book text-lg">{t('import.dropTitle')}</p>
        <p className="mt-1 text-[13px] text-ink-muted">{t('import.dropHint')}</p>
      </div>
    </div>
  )
}

/** 书架上的一本书：封面 + 书名 + 一行副标题 + 存档数。 */
function ShelfBook({ work }: { work: WorkSummary }) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const actions = useVersionActions(work.id)
  const rename = useRenameWork()
  const [renaming, setRenaming] = useState(false)

  const remove = async () => {
    if (!(await confirmAction(t('library.removeConfirm', { title: work.title, count: work.save_count })))) return
    try {
      await actions.remove.mutateAsync(undefined)
      await qc.invalidateQueries({ queryKey: qk.works })
      await qc.invalidateQueries({ queryKey: qk.recentSaves })
      toast(t('library.removed'))
    } catch (e) {
      toast.error(errorMessage(e))
    }
  }

  return (
    <div className="group relative">
      <Link to={`/works/${encodeURIComponent(work.id)}`} className="block rounded-sm outline-offset-4">
        <WorkCover
          id={work.id}
          title={work.title}
          dev={work.latest.is_dev}
          image={work.cover_url}
          className="transition-[translate,box-shadow] duration-300 ease-out group-hover:-translate-y-1 group-hover:shadow-lift"
        />
        <p className="mt-4 truncate font-serif text-[15px] leading-6 font-semibold tracking-[0.04em] text-ink">{work.title}</p>
        <p className="truncate text-xs leading-5 text-ink-muted">{work.subtitle || ' '}</p>
        <p className="mt-1 text-xs leading-5 text-ink-faint">{t('library.saveCount', { count: work.save_count })}</p>
      </Link>
      <div className="absolute top-2 right-2 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('library.rename')}
          title={t('library.rename')}
          onClick={() => setRenaming(true)}
          className="size-8 rounded-full bg-paper/85 text-ink-muted shadow-sm hover:text-ink"
        >
          <Pencil className="size-4" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          aria-label={t('library.remove')}
          title={t('library.remove')}
          disabled={actions.remove.isPending}
          onClick={() => void remove()}
          className="size-8 rounded-full bg-paper/85 text-ink-muted shadow-sm hover:text-danger"
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
      <WorkRenameDialog
        open={renaming}
        title={work.title}
        onClose={() => setRenaming(false)}
        onSubmit={(title) =>
          rename.mutate(
            { id: work.id, title },
            {
              onSuccess: () => {
                setRenaming(false)
                toast(t('library.renamed'))
              },
              onError: (e) => toast.error(errorMessage(e)),
            },
          )
        }
      />
    </div>
  )
}

/** 继续阅读：一册微缩封面 + 存档名 + 回合与时间，点击直接回到阅读页。 */
function SaveSlip({ save }: { save: Save }) {
  const { t } = useTranslation()
  return (
    <Link
      to={`/play/${save.id}`}
      className="group flex h-full items-stretch gap-4 rounded-xl border border-rule bg-paper-raised p-3 pr-4 transition-colors hover:border-rule-strong hover:bg-hover"
    >
      <WorkCover id={save.work_id} title={save.work_title} compact className="w-12 shrink-0" />
      <span className="flex min-w-0 flex-1 flex-col justify-between">
        <span className="min-w-0">
          <span className="block truncate text-xs leading-5 text-ink-faint">{save.work_title}</span>
          <span className="block truncate font-serif text-[15px] leading-6 font-semibold text-ink">{saveTitle(save)}</span>
        </span>
        <span className="flex items-center justify-between gap-3 text-xs leading-5 text-ink-muted">
          <span className="tabular">{t('library.turnNo', { n: save.turn_count })}</span>
          <span className="truncate">{formatRelative(save.last_played_at)}</span>
        </span>
      </span>
    </Link>
  )
}

function ShelfSkeleton() {
  return (
    <div className={SHELF_GRID}>
      {Array.from({ length: 4 }, (_, i) => (
        <div key={i}>
          <Skeleton className="aspect-[5/7] w-full rounded-sm" />
          <Skeleton className="mt-4 h-4 w-2/3" />
          <Skeleton className="mt-2 h-3 w-1/2" />
        </div>
      ))}
    </div>
  )
}

function EmptyShelf({ onImport }: { onImport: () => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col items-center rounded-2xl border border-dashed border-rule-strong px-6 py-20 text-center">
      <div aria-hidden className="mb-6 h-24 w-[68px] rounded-[2px_4px_4px_2px] border border-dashed border-ink-faint/60" />
      <p className="title-book text-lg">{t('library.emptyTitle')}</p>
      <p className="mt-2 max-w-sm text-ink-muted">{t('library.emptyBody')}</p>
      <Button className="mt-6" onClick={onImport}>
        <Plus />
        {t('library.import')}
      </Button>
    </div>
  )
}
