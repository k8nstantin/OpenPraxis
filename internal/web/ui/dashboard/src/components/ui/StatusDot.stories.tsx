import type { Meta, StoryObj } from '@storybook/react';
import { StatusDot } from './StatusDot';

const meta: Meta<typeof StatusDot> = { title: 'UI / StatusDot', component: StatusDot };
export default meta;

export const Open: StoryObj<typeof StatusDot> = { args: { status: 'open' } };
export const Running: StoryObj<typeof StatusDot> = { args: { status: 'running' } };
export const Failed: StoryObj<typeof StatusDot> = { args: { status: 'failed' } };
export const Completed: StoryObj<typeof StatusDot> = { args: { status: 'completed' } };
export const WithLabel: StoryObj<typeof StatusDot> = { args: { status: 'running', label: 'task is running' } };
