import type { Meta, StoryObj } from '@storybook/react';
import { Button } from './Button';

const meta: Meta<typeof Button> = {
  title: 'UI/Button',
  component: Button,
  args: { children: 'Save changes' },
  argTypes: {
    variant: { control: 'select', options: ['primary', 'secondary', 'ghost', 'danger'] },
    size: { control: 'select', options: ['sm', 'md', 'lg'] },
  },
  tags: ['autodocs'],
};
export default meta;

export const Primary: StoryObj<typeof Button> = { args: { variant: 'primary' } };
export const Secondary: StoryObj<typeof Button> = { args: { variant: 'secondary' } };
export const Ghost: StoryObj<typeof Button> = { args: { variant: 'ghost' } };
export const Danger: StoryObj<typeof Button> = { args: { variant: 'danger', children: 'Delete' } };
export const Loading: StoryObj<typeof Button> = { args: { variant: 'primary', loading: true } };
export const Disabled: StoryObj<typeof Button> = { args: { variant: 'primary', disabled: true } };
