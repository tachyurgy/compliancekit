import type { Resolver, FieldValues } from 'react-hook-form';
import type { ZodType } from 'zod';

/**
 * Minimal Zod <-> react-hook-form resolver. Avoids a runtime dep on
 * `@hookform/resolvers` so installs stay lean.
 */
export function zodResolver<T extends FieldValues>(schema: ZodType<T>): Resolver<T> {
  return async (values) => {
    const result = schema.safeParse(values);
    if (result.success) {
      return { values: result.data, errors: {} };
    }
    const errors: Record<string, { type: string; message: string }> = {};
    for (const issue of result.error.issues) {
      const path = issue.path.join('.') || '_form';
      if (!errors[path]) {
        errors[path] = { type: issue.code, message: issue.message };
      }
    }
    return { values: {} as T, errors: errors as never };
  };
}
