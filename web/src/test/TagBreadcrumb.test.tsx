/**
 * TagBreadcrumb component tests.
 */
import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { TagBreadcrumb } from '../components/TagBreadcrumb.tsx'
import type { FileField } from '../api/types.ts'

const directField: FileField = {
  field_type_id: 1,
  field_type_name: '引擎',
  value_id: 3,
  value: '引擎A',
  source_path: 'Projects/',
  inherited: false,
  breadcrumb: [],
}

const inheritedField: FileField = {
  field_type_id: 2,
  field_type_name: '類型',
  value_id: 7,
  value: '動作',
  source_path: 'Projects/',
  inherited: true,
  breadcrumb: ['遊戲', '類型'],
}

describe('TagBreadcrumb', () => {
  it('renders empty state when no fields', () => {
    render(<TagBreadcrumb fields={[]} />)
    expect(screen.getByText('（無標籤）')).toBeInTheDocument()
  })

  it('renders field type name and value', () => {
    render(<TagBreadcrumb fields={[directField]} />)
    expect(screen.getByText('引擎')).toBeInTheDocument()
    expect(screen.getByText('引擎A')).toBeInTheDocument()
  })

  it('shows "繼承" badge for inherited fields', () => {
    render(<TagBreadcrumb fields={[inheritedField]} />)
    expect(screen.getByText('繼承')).toBeInTheDocument()
  })

  it('does not show "繼承" badge for direct fields', () => {
    render(<TagBreadcrumb fields={[directField]} />)
    expect(screen.queryByText('繼承')).not.toBeInTheDocument()
  })

  it('renders breadcrumb crumbs for inherited field', () => {
    render(<TagBreadcrumb fields={[inheritedField]} />)
    expect(screen.getByText('遊戲')).toBeInTheDocument()
    // '類型' appears in both typeName span and breadcrumb crumb — use getAllByText
    const matches = screen.getAllByText('類型')
    expect(matches.length).toBeGreaterThanOrEqual(2)
  })

  it('shows remove button only for direct (non-inherited) fields when onRemove provided', () => {
    const onRemove = vi.fn()
    render(<TagBreadcrumb fields={[directField, inheritedField]} onRemove={onRemove} />)
    // Only direct field gets a remove button
    const removeBtns = screen.getAllByRole('button', { name: /移除/ })
    expect(removeBtns).toHaveLength(1)
    expect(removeBtns[0]).toHaveAttribute('aria-label', '移除 引擎')
  })

  it('calls onRemove with correct fieldTypeId when remove button clicked', () => {
    const onRemove = vi.fn()
    render(<TagBreadcrumb fields={[directField]} onRemove={onRemove} />)
    fireEvent.click(screen.getByRole('button', { name: '移除 引擎' }))
    expect(onRemove).toHaveBeenCalledWith(1)
  })

  it('does not render remove button when onRemove not provided', () => {
    render(<TagBreadcrumb fields={[directField]} />)
    expect(screen.queryByRole('button', { name: /移除/ })).not.toBeInTheDocument()
  })

  it('renders multiple fields', () => {
    render(<TagBreadcrumb fields={[directField, inheritedField]} />)
    // '引擎' and '類型' appear as field type names
    expect(screen.getByText('引擎')).toBeInTheDocument()
    // '類型' appears in typeName AND crumb — at least once
    const typeMatches = screen.getAllByText('類型')
    expect(typeMatches.length).toBeGreaterThanOrEqual(1)
  })

  it('applies inherited title attribute with source_path', () => {
    render(<TagBreadcrumb fields={[inheritedField]} />)
    const chip = screen.getByTitle('繼承自 Projects/')
    expect(chip).toBeInTheDocument()
  })

  it('applies "直接套用" title for direct fields', () => {
    render(<TagBreadcrumb fields={[directField]} />)
    expect(screen.getByTitle('直接套用')).toBeInTheDocument()
  })

  it('renders the value chip text', () => {
    render(<TagBreadcrumb fields={[inheritedField]} />)
    expect(screen.getByText('動作')).toBeInTheDocument()
  })
})
