import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { PickerDialog, type PickerDialogItem } from '../PickerDialog';

const items: PickerDialogItem[] = [
  { id: 'a', search: 'alpha apples', render: <span>Alpha</span> },
  { id: 'b', search: 'beta bananas', render: <span>Beta</span> },
  { id: 'c', search: 'gamma grapes', render: <span>Gamma</span> },
];

describe('PickerDialog', () => {
  it('renders all items when filter is empty', () => {
    render(
      <PickerDialog
        open
        items={items}
        title="Pick"
        onOpenChange={() => {}}
        onPick={() => {}}
      />,
    );
    expect(screen.getByText('Alpha')).toBeTruthy();
    expect(screen.getByText('Beta')).toBeTruthy();
    expect(screen.getByText('Gamma')).toBeTruthy();
  });

  it('filters case-insensitively across the search blob', () => {
    render(
      <PickerDialog
        open
        items={items}
        title="Pick"
        onOpenChange={() => {}}
        onPick={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText('Filter list'), { target: { value: 'BANAN' } });
    expect(screen.queryByText('Alpha')).toBeNull();
    expect(screen.getByText('Beta')).toBeTruthy();
    expect(screen.queryByText('Gamma')).toBeNull();
  });

  it('fires onPick with the item id on click', () => {
    const onPick = vi.fn();
    render(
      <PickerDialog
        open
        items={items}
        title="Pick"
        onOpenChange={() => {}}
        onPick={onPick}
      />,
    );
    fireEvent.click(screen.getByText('Beta'));
    expect(onPick).toHaveBeenCalledWith('b');
  });

  it('shows "no items match" when filter excludes everything', () => {
    render(
      <PickerDialog
        open
        items={items}
        title="Pick"
        onOpenChange={() => {}}
        onPick={() => {}}
      />,
    );
    fireEvent.change(screen.getByLabelText('Filter list'), { target: { value: 'zzz' } });
    expect(screen.getByText('No items match the filter.')).toBeTruthy();
  });
});
