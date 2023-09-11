/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
import { ComponentMeta } from '@storybook/react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import TextField from '@pyroscope/ui/Form/TextField';
import Button from '@pyroscope/ui/Button';
import * as z from 'zod';
import '../sass/profile.scss';

export default {
  title: 'Examples/Form',
} as ComponentMeta<any>;

const schema = z.object({
  name: z.string().min(1, { message: 'Required' }),
  age: z.number().min(10),
});
export function ExampleForm() {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm({
    resolver: zodResolver(schema),
  });

  return (
    <form onSubmit={handleSubmit((d) => alert(JSON.stringify(d)))}>
      <TextField
        {...register('name')}
        label="Name"
        errorMessage={errors.name?.message}
      />

      <TextField
        label="Age"
        type="number"
        {...register('age', { valueAsNumber: true })}
        errorMessage={errors.age?.message}
      />

      <Button type="submit" kind="secondary">
        Submit
      </Button>
    </form>
  );
}
