/**
 * SearchFilterBar component tests.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { SearchFilterBar } from '../components/SearchFilterBar.tsx'
import type { FieldType, SearchFilter } from '../api/types.ts'

// Mock fieldValuesApi.search
vi.mock('../api/client.ts', () => ({
  fieldValuesApi: {
    search: vi.fn(),
  },
}))

import { fieldValuesApi } from '../api/client.ts'

const fieldTypes: FieldType[] = [
  { id: 1, name: '引擎', allow_multi: false },
  { id: 2, name: '類型', allow_multi: true },
]

const emptyFilters: SearchFilter[] = []

const activeFilter: SearchFilter = {
  field_type_id: 1,
  field_value_id: 5,
  strict: false,
}

function setup(filters = emptyFilters, onChange = vi.fn()) {
  const utils = render(
    <SearchFilterBar
      fieldTypes={fieldTypes}
      filters={filters}
      onChange={onChange}
    />,
  )
  return { ...utils, onChange }
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('SearchFilterBar', () => {
  it('renders field type select with options', () => {
    setup()
    const select = screen.getByLabelText('選擇欄位種類')
    expect(select).toBeInTheDocument()
    // options inside select
    expect(screen.getAllByText('引擎').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('類型').length).toBeGreaterThanOrEqual(1)
  })

  it('search button is disabled when no type selected', () => {
    setup()
    expect(screen.getByRole('button', { name: '搜尋' })).toBeDisabled()
  })

  it('search button is disabled when query is empty', () => {
    setup()
    fireEvent.change(screen.getByLabelText('選擇欄位種類'), { target: { value: '1' } })
    expect(screen.getByRole('button', { name: '搜尋' })).toBeDisabled()
  })

  it('input is disabled when no type selected', () => {
    setup()
    expect(screen.getByLabelText('搜尋標籤值')).toBeDisabled()
  })

  it('calls fieldValuesApi.search when search button clicked', async () => {
    const mockSearch = vi.mocked(fieldValuesApi.search)
    mockSearch.mockResolvedValue([
      { id: 5, field_type_id: 1, parent_id: null, value: '引擎A', path: '/5/' },
    ])

    setup()
    fireEvent.change(screen.getByLabelText('選擇欄位種類'), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText('搜尋標籤值'), { target: { value: '引擎查詢' } })
    fireEvent.click(screen.getByRole('button', { name: '搜尋' }))

    await waitFor(() => {
      expect(mockSearch).toHaveBeenCalledWith('引擎查詢', 1)
    })
  })

  it('shows search results after search', async () => {
    const mockSearch = vi.mocked(fieldValuesApi.search)
    mockSearch.mockResolvedValue([
      { id: 5, field_type_id: 1, parent_id: null, value: '引擎A', path: '/5/' },
    ])

    setup()
    fireEvent.change(screen.getByLabelText('選擇欄位種類'), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText('搜尋標籤值'), { target: { value: '引擎查詢' } })
    fireEvent.click(screen.getByRole('button', { name: '搜尋' }))

    await waitFor(() => {
      expect(screen.getByText('引擎A')).toBeInTheDocument()
    })
  })

  it('calls onChange when a value is selected', async () => {
    const mockSearch = vi.mocked(fieldValuesApi.search)
    mockSearch.mockResolvedValue([
      { id: 5, field_type_id: 1, parent_id: null, value: '引擎A', path: '/5/' },
    ])

    const onChange = vi.fn()
    render(
      <SearchFilterBar
        fieldTypes={fieldTypes}
        filters={[]}
        onChange={onChange}
      />,
    )
    fireEvent.change(screen.getByLabelText('選擇欄位種類'), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText('搜尋標籤值'), { target: { value: '引擎查詢' } })
    fireEvent.click(screen.getByRole('button', { name: '搜尋' }))

    await waitFor(() => screen.getByText('引擎A'))
    fireEvent.click(screen.getByText('引擎A'))

    expect(onChange).toHaveBeenCalledWith([
      { field_type_id: 1, field_value_id: 5, strict: false },
    ])
  })

  it('renders active filter chips with value id', () => {
    setup([activeFilter])
    // chip shows '#5'
    expect(screen.getByText('#5')).toBeInTheDocument()
    // chipType shows field type name — may appear in both chip and option
    const matches = screen.getAllByText('引擎')
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it('calls onChange with filter removed when × clicked', () => {
    const { onChange } = setup([activeFilter])
    fireEvent.click(screen.getByRole('button', { name: '移除篩選條件' }))
    expect(onChange).toHaveBeenCalledWith([])
  })

  it('toggles strict mode on filter chip via title', () => {
    const { onChange } = setup([activeFilter])
    // The strict button has title (not aria-label), use getByTitle
    const strictBtn = screen.getByTitle('模糊模式（點擊開啟嚴格）')
    fireEvent.click(strictBtn)
    expect(onChange).toHaveBeenCalledWith([
      { ...activeFilter, strict: true },
    ])
  })

  it('prevents adding exact duplicate filter', async () => {
    const mockSearch = vi.mocked(fieldValuesApi.search)
    mockSearch.mockResolvedValue([
      { id: 5, field_type_id: 1, parent_id: null, value: '引擎A', path: '/5/' },
    ])
    const onChange = vi.fn()
    render(
      <SearchFilterBar
        fieldTypes={fieldTypes}
        filters={[activeFilter]}
        onChange={onChange}
      />,
    )
    fireEvent.change(screen.getByLabelText('選擇欄位種類'), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText('搜尋標籤值'), { target: { value: '引擎查詢' } })
    fireEvent.click(screen.getByRole('button', { name: '搜尋' }))

    await waitFor(() => screen.getByText('引擎A'))
    fireEvent.click(screen.getByText('引擎A'))

    // Should not call onChange since it's a duplicate
    expect(onChange).not.toHaveBeenCalled()
  })
})
