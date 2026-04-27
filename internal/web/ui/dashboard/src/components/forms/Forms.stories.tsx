import type { Meta, StoryObj } from '@storybook/react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { FormField } from './FormField';
import { FormSection } from './FormSection';
import { FormActions } from './FormActions';
import { FormError } from './FormError';
import { Button } from '../ui/Button';
import { productCreateSchema, type ProductCreateInput } from '../../schemas/product';

const meta: Meta = { title: 'Forms/ProductForm', tags: ['autodocs'] };
export default meta;

export const Basic: StoryObj = {
  render: () => {
    const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm<ProductCreateInput>({
      resolver: zodResolver(productCreateSchema),
      defaultValues: { title: '', description: '', status: 'active' },
    });
    return (
      <form
        onSubmit={handleSubmit((v) => {
          console.log('submit', v);
        })}
        style={{ maxWidth: 480 }}
      >
        <FormSection title="Product details" description="Required to create a new product.">
          <FormField label="Title" required {...register('title')} error={errors.title?.message} />
          <FormField as="textarea" label="Description" rows={4} {...register('description')} error={errors.description?.message} />
        </FormSection>
        <FormError>{errors.root?.message}</FormError>
        <FormActions>
          <Button>Cancel</Button>
          <Button variant="primary" type="submit" loading={isSubmitting}>Create</Button>
        </FormActions>
      </form>
    );
  },
};
