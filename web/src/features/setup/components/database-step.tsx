/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { CheckCircle2, Database, HardDrive, Loader2, Server } from 'lucide-react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { handleServerError } from '@/lib/handle-server-error'

import { saveDatabaseConfig } from '../api'
import type { DatabaseConfig, SetupStatus } from '../types'

interface DatabaseStepProps {
  status?: SetupStatus
  savedConfig?: DatabaseConfig | null
  onSave: (config: DatabaseConfig) => void
}

const DATABASE_OPTIONS = [
  { value: 'sqlite', labelKey: 'SQLite' },
  { value: 'mysql', labelKey: 'MySQL' },
  { value: 'postgres', labelKey: 'PostgreSQL' },
] as const

const DB_META: Record<
  string,
  {
    icon: typeof Database
    descriptionKey: string
    variant: 'info' | 'success' | 'warning'
    alertKey?: string
  }
> = {
  sqlite: {
    icon: Database,
    descriptionKey:
      'SQLite stores all data in a single file. Make sure that file is persisted when running in containers.',
    variant: 'warning',
  },
  mysql: {
    icon: Server,
    descriptionKey:
      'MySQL is a production-ready relational database. Keep your credentials secure.',
    variant: 'success',
    alertKey: 'MySQL detected',
  },
  postgres: {
    icon: Server,
    descriptionKey:
      'PostgreSQL offers advanced reliability and data integrity for production workloads.',
    variant: 'success',
    alertKey: 'PostgreSQL detected',
  },
}

const DEFAULT_DB_CONFIG: DatabaseConfig = {
  databaseType: 'sqlite',
  host: '127.0.0.1',
  port: 3306,
  username: 'root',
  password: '',
  database: 'new_api',
}

export function DatabaseStep({ status, savedConfig, onSave }: DatabaseStepProps) {
  const { t } = useTranslation()

  // The backend reports custom_database=true only when SQL_DSN (or a
  // persisted config file) is set. When false the system runs on the
  // built-in default SQLite, so we offer the configuration form.
  const hasCustomDB = Boolean(status?.custom_database)

  // Config saved in this wizard session (state lifted to the wizard so it
  // survives step navigation).
  const savedType = savedConfig?.databaseType ?? null

  const [saving, setSaving] = useState(false)

  const showForm = !hasCustomDB && !savedType

  const form = useForm<DatabaseConfig>({
    defaultValues: DEFAULT_DB_CONFIG,
    mode: 'onBlur',
  })

  const mutation = useMutation({
    mutationFn: saveDatabaseConfig,
    onSuccess: (response) => {
      if (response.success) {
        toast.success(t('Database configuration saved. Please restart the server to apply.'))
        onSave(form.getValues())
      } else {
        handleServerError(response, t('Failed to save database configuration'))
      }
    },
    onError: (error) => {
      handleServerError(error, t('Failed to save database configuration'))
    },
  })

  const handleSubmit = (values: DatabaseConfig) => {
    if (values.databaseType === 'sqlite') {
      onSave(values)
      return
    }
    if (!values.host || !values.database) {
      toast.error(t('Please fill in all required fields'))
      return
    }
    setSaving(true)
    mutation.mutate(values)
    setSaving(false)
  }

  const activeType = form.watch('databaseType')
  const displayType = (savedType || status?.database_type || '').toLowerCase()
  const meta = DB_META[displayType] ?? DB_META.sqlite

  return (
    <div className='space-y-4'>
      {showForm ? (
        <Form {...form}>
          <div className='space-y-6'>
          <FormField
            control={form.control}
            name='databaseType'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Database type')}</FormLabel>
                <Select
                  onValueChange={field.onChange}
                  defaultValue={field.value}
                  value={field.value}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {DATABASE_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {t(opt.labelKey)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormDescription>
                  {t('Choose the database engine for your data')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {activeType !== 'sqlite' && (
            <div className='space-y-4 rounded-lg border p-4'>
              <FormField
                control={form.control}
                name='host'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Host')}</FormLabel>
                    <FormControl>
                      <Input placeholder='127.0.0.1' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='port'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Port')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        placeholder={
                          activeType === 'mysql' ? '3306' : '5432'
                        }
                        {...field}
                        onChange={(e) =>
                          field.onChange(parseInt(e.target.value, 10) || 0)
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='username'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Username')}</FormLabel>
                    <FormControl>
                      <Input placeholder='root' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='password'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Password')}</FormLabel>
                    <FormControl>
                      <Input type='password' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='database'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Database name')}</FormLabel>
                    <FormControl>
                      <Input placeholder='new_api' {...field} />
                    </FormControl>
                    <FormDescription>
                      {t('The database will be created if it does not exist')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          )}

          <div className='flex justify-end'>
            <Button
              type='button'
              variant='outline'
              onClick={() => form.handleSubmit(handleSubmit)()}
              disabled={saving || mutation.isPending}
            >
              {saving || mutation.isPending ? (
                <>
                  <Loader2 className='mr-2 size-4 animate-spin' />
                  {t('Saving...')}
                </>
              ) : (
                t('Save configuration')
              )}
            </Button>
          </div>
          </div>
        </Form>
      ) : null}

      {/* Show current or saved database info */}
      {!showForm && (
        <div className='bg-card flex items-center justify-between rounded-lg border p-4'>
          <div className='space-y-1'>
            <p className='text-muted-foreground text-sm font-medium'>
              {t('Database configuration')}
            </p>
            <p className='text-foreground text-base font-semibold'>
              {displayType ? displayType.toUpperCase() : t('Unknown')}
            </p>
            <p className='text-muted-foreground text-sm'>
              {t(
                meta?.descriptionKey ??
                  'The setup wizard will use this database during initialization.'
              )}
            </p>
          </div>
          <StatusBadge
            label={displayType ? displayType.toUpperCase() : t('Unknown')}
            variant={meta?.variant ?? 'info'}
            className='cursor-default'
            copyable={false}
            icon={meta?.icon ?? Database}
          />
        </div>
      )}

      {displayType === 'mysql' && (
        <Alert className='border-emerald-200 bg-emerald-50 dark:border-emerald-900/60 dark:bg-emerald-950/40'>
          <AlertTitle className='flex items-center gap-2'>
            <CheckCircle2 className='size-4 text-emerald-500' />
            {t('MySQL detected')}
          </AlertTitle>
          <AlertDescription>
            {t(
              'MySQL is production ready. Ensure automated backups and a dedicated user with the minimal required privileges are configured.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {displayType === 'postgres' && (
        <Alert className='border-sky-200 bg-sky-50 dark:border-sky-900/60 dark:bg-sky-950/40'>
          <AlertTitle className='flex items-center gap-2'>
            <CheckCircle2 className='size-4 text-sky-500' />
            {t('PostgreSQL detected')}
          </AlertTitle>
          <AlertDescription>
            {t(
              'PostgreSQL offers strong reliability guarantees. Double check your maintenance window and retention policies before going live.'
            )}
          </AlertDescription>
        </Alert>
      )}

      {!showForm && displayType === 'sqlite' && (
        <Alert className='border-amber-200 bg-amber-50 dark:border-amber-900/60 dark:bg-amber-950/40'>
          <AlertTitle className='flex items-center gap-2'>
            <HardDrive className='size-4 text-amber-500' />
            {t('Persist your data file')}
          </AlertTitle>
          <AlertDescription>
            <p>
              {t(
                'When running in containers or ephemeral environments, ensure the SQLite file is mapped to persistent storage to avoid data loss on restart.'
              )}
            </p>
          </AlertDescription>
        </Alert>
      )}

      {savedType && savedType !== 'sqlite' && (
        <Alert className='border-blue-200 bg-blue-50 dark:border-blue-900/60 dark:bg-blue-950/40'>
          <AlertTitle className='flex items-center gap-2'>
            <Server className='size-4 text-blue-500' />
            {t('Restart required')}
          </AlertTitle>
          <AlertDescription>
            <p className='mb-2'>
              {t('Database configuration saved. Please restart the server to apply.')}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Stop the server, then start it again. The setup wizard will detect your database automatically.'
              )}
            </p>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
