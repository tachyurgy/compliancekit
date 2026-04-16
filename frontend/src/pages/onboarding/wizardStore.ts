import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type StateCode = 'CA' | 'TX' | 'FL';
export type LicenseType = 'center' | 'family_home';

export interface StaffDraft {
  firstName: string;
  lastName: string;
  email?: string;
  role: 'director' | 'lead_teacher' | 'assistant' | 'aide' | 'cook' | 'other';
}

export interface ChildDraft {
  firstName: string;
  lastName: string;
  dateOfBirth: string;
  parentEmail?: string;
}

export interface WizardData {
  stateCode: StateCode | null;
  licenseType: LicenseType | null;
  licenseNumber: string;
  name: string;
  address1: string;
  address2: string;
  city: string;
  stateRegion: string;
  postalCode: string;
  capacity: number | null;
  minAgeMonths: number | null;
  maxAgeMonths: number | null;
  staff: StaffDraft[];
  children: ChildDraft[];
}

interface WizardStore extends WizardData {
  currentStep: number;
  setCurrentStep: (n: number) => void;
  setField: <K extends keyof WizardData>(key: K, value: WizardData[K]) => void;
  patch: (partial: Partial<WizardData>) => void;
  addStaff: (s: StaffDraft) => void;
  removeStaff: (index: number) => void;
  addChild: (c: ChildDraft) => void;
  removeChild: (index: number) => void;
  reset: () => void;
}

const initial: WizardData = {
  stateCode: null,
  licenseType: null,
  licenseNumber: '',
  name: '',
  address1: '',
  address2: '',
  city: '',
  stateRegion: '',
  postalCode: '',
  capacity: null,
  minAgeMonths: null,
  maxAgeMonths: null,
  staff: [],
  children: [],
};

export const useWizardStore = create<WizardStore>()(
  persist(
    (set) => ({
      ...initial,
      currentStep: 0,
      setCurrentStep: (n) => set({ currentStep: n }),
      setField: (key, value) => set({ [key]: value } as Partial<WizardStore>),
      patch: (partial) => set(partial as Partial<WizardStore>),
      addStaff: (s) => set((state) => ({ staff: [...state.staff, s] })),
      removeStaff: (index) =>
        set((state) => ({ staff: state.staff.filter((_, i) => i !== index) })),
      addChild: (c) => set((state) => ({ children: [...state.children, c] })),
      removeChild: (index) =>
        set((state) => ({ children: state.children.filter((_, i) => i !== index) })),
      reset: () => set({ ...initial, currentStep: 0 }),
    }),
    {
      name: 'compliancekit-onboarding',
      version: 1,
    },
  ),
);

export const WIZARD_STEPS = [
  { path: 'state', label: 'State' },
  { path: 'license', label: 'License' },
  { path: 'facility', label: 'Facility' },
  { path: 'staff', label: 'Staff' },
  { path: 'children', label: 'Children' },
  { path: 'review', label: 'Review' },
] as const;

export type WizardStepPath = (typeof WIZARD_STEPS)[number]['path'];
