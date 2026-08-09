'use client'

import { useState, useCallback, Children, isValidElement, cloneElement } from 'react'

interface Props {
  children: React.ReactNode
}

interface ElementProps {
  children?: React.ReactNode
  style?: React.CSSProperties
  [key: string]: unknown
}

/**
 * SortableTable wraps a markdown-rendered <table> to add click-to-sort on
 * column headers. Numeric columns sort numerically; everything else sorts
 * lexicographically. An active sort column shows a directional indicator
 * and a row-count footer is always displayed.
 */
export default function SortableTable({ children }: Props) {
  const [sortCol, setSortCol] = useState<number>(-1)
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')

  // Recursively extract the plain-text content from a React element tree.
  const getTextContent = (node: React.ReactNode): string => {
    if (typeof node === 'string') return node
    if (typeof node === 'number') return String(node)
    if (Array.isArray(node)) return node.map(getTextContent).join('')
    if (isValidElement(node)) {
      const p = node.props as ElementProps
      if (p.children) return getTextContent(p.children)
    }
    return ''
  }

  // Walk the children tree to find <tbody> and collect its <tr> rows.
  const getRows = (): { element: React.ReactElement; cells: string[] }[] => {
    const rows: { element: React.ReactElement; cells: string[] }[] = []
    Children.forEach(children, (child) => {
      if (!isValidElement(child)) return
      if (typeof child.type === 'string' && child.type === 'tbody') {
        const tbodyProps = child.props as ElementProps
        Children.forEach(tbodyProps.children, (tr) => {
          if (!isValidElement(tr)) return
          const trProps = tr.props as ElementProps
          const cells: string[] = []
          Children.forEach(trProps.children, (td) => {
            cells.push(getTextContent(td))
          })
          rows.push({ element: tr, cells })
        })
      }
    })
    return rows
  }

  const handleSort = useCallback(
    (colIndex: number) => {
      if (sortCol === colIndex) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
      } else {
        setSortCol(colIndex)
        setSortDir('asc')
      }
    },
    [sortCol],
  )

  const rows = getRows()

  const sortedRows =
    sortCol >= 0
      ? [...rows].sort((a, b) => {
          const aVal = a.cells[sortCol] ?? ''
          const bVal = b.cells[sortCol] ?? ''

          // Attempt numeric comparison first (strip non-numeric chars like $, %, etc.)
          const aNum = parseFloat(aVal.replace(/[^0-9.\-+eE]/g, ''))
          const bNum = parseFloat(bVal.replace(/[^0-9.\-+eE]/g, ''))
          if (!isNaN(aNum) && !isNaN(bNum)) {
            return sortDir === 'asc' ? aNum - bNum : bNum - aNum
          }

          // Fall back to locale-aware string comparison
          return sortDir === 'asc'
            ? aVal.localeCompare(bVal)
            : bVal.localeCompare(aVal)
        })
      : rows

  // Rebuild the table with sorted tbody rows and clickable thead headers.
  const newChildren = Children.map(children, (child) => {
    if (!isValidElement(child)) return child

    // --- thead: attach click handlers + sort indicator to each <th> ---
    if (typeof child.type === 'string' && child.type === 'thead') {
      const theadProps = child.props as ElementProps
      const newThead = cloneElement(
        child,
        {},
        Children.map(theadProps.children, (tr) => {
          if (!isValidElement(tr)) return tr
          const trProps = tr.props as ElementProps
          return cloneElement(
            tr,
            {},
            Children.map(trProps.children, (th, i: number) => {
              if (!isValidElement(th)) return th
              const thProps = th.props as ElementProps
              return cloneElement(th as React.ReactElement<ElementProps>, {
                style: {
                  ...(thProps.style || {}),
                  cursor: 'pointer',
                  userSelect: 'none' as const,
                },
                onClick: () => handleSort(i),
                children: (
                  <>
                    {thProps.children}
                    {sortCol === i && (
                      <span
                        style={{ marginLeft: 4, fontSize: 10, opacity: 0.7 }}
                        aria-label={sortDir === 'asc' ? 'sorted ascending' : 'sorted descending'}
                      >
                        {sortDir === 'asc' ? '\u25B2' : '\u25BC'}
                      </span>
                    )}
                  </>
                ),
              })
            }),
          )
        }),
      )
      return newThead
    }

    // --- tbody: replace rows with the sorted order ---
    if (typeof child.type === 'string' && child.type === 'tbody') {
      return cloneElement(
        child,
        {},
        sortedRows.map((r, i) => cloneElement(r.element, { key: i })),
      )
    }

    return child
  })

  return (
    <div>
      <div style={{ overflowX: 'auto', maxWidth: '100%' }}>
        <table>{newChildren}</table>
      </div>
      <div
        style={{
          fontSize: 11,
          color: 'var(--text-muted)',
          marginTop: 4,
          fontFamily: 'inherit',
        }}
      >
        Showing {rows.length} row{rows.length !== 1 ? 's' : ''}
      </div>
    </div>
  )
}
