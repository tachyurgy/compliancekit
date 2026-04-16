import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { CreditCard, Building, AlertTriangle } from 'lucide-react';

import PageHeader from '@/components/common/PageHeader';
import Card from '@/components/common/Card';
import Button from '@/components/common/Button';
import LoadingSpinner from '@/components/common/LoadingSpinner';
import { providersApi } from '@/api/providers';
import { useSession } from '@/hooks/useSession';

export default function Settings(): JSX.Element {
  const user = useSession((s) => s.user);
  const { data: provider, isLoading, isError, refetch } = useQuery({
    queryKey: ['provider'],
    queryFn: () => providersApi.getProvider(),
  });

  return (
    <div>
      <PageHeader
        title="Settings"
        description="Manage your facility information and account."
      />

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card
          title="Facility"
          description="Address, capacity, and licensing basics."
        >
          {isLoading ? (
            <LoadingSpinner />
          ) : isError ? (
            <div className="flex items-start gap-2 text-critical-700">
              <AlertTriangle className="h-5 w-5 mt-0.5" />
              <div>
                <div className="font-medium">Couldn't load your facility</div>
                <Button
                  size="sm"
                  variant="secondary"
                  className="mt-2"
                  onClick={() => refetch()}
                >
                  Retry
                </Button>
              </div>
            </div>
          ) : provider ? (
            <dl className="text-sm space-y-2">
              <Row label="Name" value={provider.name} icon={<Building className="h-4 w-4" />} />
              <Row label="State" value={provider.stateCode} />
              <Row
                label="License type"
                value={provider.licenseType === 'center' ? 'Child Care Center' : 'Family Home'}
              />
              <Row label="License #" value={provider.licenseNumber ?? '—'} />
              <Row
                label="Address"
                value={`${provider.address1}, ${provider.city}, ${provider.stateRegion} ${provider.postalCode}`}
              />
              <Row label="Capacity" value={String(provider.capacity)} />
            </dl>
          ) : null}
        </Card>

        <Card title="Account">
          <dl className="text-sm space-y-2">
            <Row label="Email" value={user?.email ?? '—'} />
            <Row label="Name" value={user?.fullName ?? '—'} />
            <Row label="Role" value={user?.role ?? '—'} />
          </dl>
        </Card>

        <Card
          title="Billing"
          description="Subscription and invoices."
          action={
            <Link to="/settings/billing">
              <Button size="sm" variant="secondary" leftIcon={<CreditCard className="h-4 w-4" />}>
                Manage
              </Button>
            </Link>
          }
        >
          <p className="text-sm text-gray-600">
            View your plan, update payment method, and access past invoices.
          </p>
        </Card>
      </div>
    </div>
  );
}

function Row({
  label,
  value,
  icon,
}: {
  label: string;
  value: React.ReactNode;
  icon?: React.ReactNode;
}): JSX.Element {
  return (
    <div className="flex items-center justify-between gap-4">
      <dt className="text-gray-500 inline-flex items-center gap-1.5">
        {icon}
        {label}
      </dt>
      <dd className="text-gray-900">{value}</dd>
    </div>
  );
}
