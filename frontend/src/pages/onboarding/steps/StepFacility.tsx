import { useNavigate } from 'react-router-dom';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import Input from '@/components/common/Input';
import { zodResolver } from '@/lib/zodResolver';
import { postalCodeSchema, requiredString, usStateRegionSchema } from '@/lib/validation';
import { useWizardStore } from '../wizardStore';

const Schema = z.object({
  name: requiredString('Facility name is required'),
  address1: requiredString('Street address is required'),
  address2: z.string().optional(),
  city: requiredString('City is required'),
  stateRegion: usStateRegionSchema,
  postalCode: postalCodeSchema,
  capacity: z
    .number({ invalid_type_error: 'Enter a capacity' })
    .int('Whole number only')
    .min(1, 'At least 1')
    .max(999, 'Unrealistic capacity'),
  minAgeMonths: z
    .number({ invalid_type_error: 'Enter a number' })
    .int()
    .min(0, 'Min 0')
    .max(216, 'Unrealistic age'),
  maxAgeMonths: z
    .number({ invalid_type_error: 'Enter a number' })
    .int()
    .min(0)
    .max(216),
});
type FormValues = z.infer<typeof Schema>;

export default function StepFacility(): JSX.Element {
  const navigate = useNavigate();
  const wizard = useWizardStore();

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(Schema),
    defaultValues: {
      name: wizard.name,
      address1: wizard.address1,
      address2: wizard.address2,
      city: wizard.city,
      stateRegion: wizard.stateRegion || (wizard.stateCode ?? ''),
      postalCode: wizard.postalCode,
      capacity: wizard.capacity ?? undefined,
      minAgeMonths: wizard.minAgeMonths ?? 0,
      maxAgeMonths: wizard.maxAgeMonths ?? 72,
    },
  });

  const onSubmit = handleSubmit((values) => {
    wizard.patch({
      name: values.name,
      address1: values.address1,
      address2: values.address2 ?? '',
      city: values.city,
      stateRegion: values.stateRegion,
      postalCode: values.postalCode,
      capacity: values.capacity,
      minAgeMonths: values.minAgeMonths,
      maxAgeMonths: values.maxAgeMonths,
    });
    navigate('/onboarding/staff');
  });

  return (
    <Card
      title="Tell us about your facility"
      description="Inspectors check the basics first — address, capacity, and ages served."
    >
      <form onSubmit={onSubmit} className="grid grid-cols-1 sm:grid-cols-2 gap-4" noValidate>
        <div className="sm:col-span-2">
          <Input label="Facility name" error={errors.name?.message} {...register('name')} />
        </div>
        <div className="sm:col-span-2">
          <Input
            label="Street address"
            error={errors.address1?.message}
            {...register('address1')}
          />
        </div>
        <div className="sm:col-span-2">
          <Input label="Address line 2" {...register('address2')} />
        </div>
        <Input label="City" error={errors.city?.message} {...register('city')} />
        <div className="grid grid-cols-2 gap-3">
          <Input
            label="State"
            maxLength={2}
            error={errors.stateRegion?.message}
            {...register('stateRegion')}
          />
          <Input
            label="ZIP code"
            error={errors.postalCode?.message}
            {...register('postalCode')}
          />
        </div>
        <Input
          label="Licensed capacity"
          type="number"
          inputMode="numeric"
          min={1}
          error={errors.capacity?.message}
          {...register('capacity', { valueAsNumber: true })}
        />
        <div className="grid grid-cols-2 gap-3">
          <Input
            label="Min age (months)"
            type="number"
            inputMode="numeric"
            error={errors.minAgeMonths?.message}
            {...register('minAgeMonths', { valueAsNumber: true })}
          />
          <Input
            label="Max age (months)"
            type="number"
            inputMode="numeric"
            error={errors.maxAgeMonths?.message}
            {...register('maxAgeMonths', { valueAsNumber: true })}
          />
        </div>

        <div className="sm:col-span-2 flex items-center justify-between pt-2">
          <Button type="button" variant="ghost" onClick={() => navigate('/onboarding/license')}>
            Back
          </Button>
          <Button type="submit">Continue</Button>
        </div>
      </form>
    </Card>
  );
}
