// REQ-041: plain-English explainers for the FL mock-inspection questions.
//
// Each explainer is keyed by the question ID from
// `backend/internal/inspection/fl_cffsp5316.go`. The IDs are stable and
// shared between the wizard ("Mock inspection") and the summary view.
//
// Voice rules:
//   - 7th-grade reading level.
//   - Calm + cheery — operators are already scared of DCF. Don't repeat it.
//   - "How to satisfy" is a numbered list of 3-5 concrete steps.
//   - "Where the rule comes from" cites the F.A.C. / F.S. line straight
//     from the verbatim handbooks in regulations/florida/.
//
// Coverage: the 15 highest-stakes questions on CF-FSP 5316 — the same
// items inspectors hit first when they walk in. The remaining 17 fall
// back to a calm "we're still writing this up" note.

export interface InspectionExplainer {
  meaning: string;
  steps: string[];
  source: string;
}

export const inspectionExplainers: Record<string, InspectionExplainer> = {
  'fl.01.ratios': {
    meaning:
      'You must have enough staff for the children present, with the ratio set by the youngest child in each group. Inspectors check this at the moment they walk in — not on average across the day.',
    steps: [
      'Know the FL ratios cold: 1:4 infants, 1:6 ones, 1:11 twos, 1:15 threes, 1:20 fours, 1:25 fives and up.',
      'Use the Ratio tab on Levelbrook to see today\'s room-by-room status.',
      'If a room is tight, float a staff member in before adding any kids.',
      'Post the ratio chart where parents and inspectors can read it.',
    ],
    source: 'F.S. §402.305(4); F.A.C. 65C-22.001(4); Handbook §3.4',
  },
  'fl.02.direct_supervision': {
    meaning:
      'Every child must be in direct line-of-sight supervision — including during naps, bathroom breaks, and outdoor play. No mirror or video monitor counts as supervision.',
    steps: [
      'Position staff so every child is visible without turning around.',
      'During naps, keep at least one adult awake and seated in the room.',
      'For bathroom routines, the door stays open and a staff member is at the threshold.',
      'On the playground, fan staff across corners — not in one cluster.',
    ],
    source: 'F.A.C. 65C-22.001(5)',
  },
  'fl.03.child_discipline': {
    meaning:
      'You need a written discipline policy that follows state rules — no corporal punishment, no humiliation, no withholding food. Every parent must receive a copy at enrollment.',
    steps: [
      'Write a policy that uses positive guidance and redirection only.',
      'Include it in the parent handbook every family signs at enrollment.',
      'Keep a signed acknowledgment in each child\'s file.',
      'Train new staff on the policy during their first week.',
    ],
    source: 'F.S. §402.305(12)',
  },
  'fl.04.minimum_standards_posted': {
    meaning:
      'The DCF "Child Care Facility Minimum Standards" summary must be posted where parents can read it without asking. Most centers put it on the parent bulletin board near the sign-in.',
    steps: [
      'Print the current Minimum Standards summary from the DCF site.',
      'Post it at parent eye-level near the sign-in or main entrance.',
      'Replace the copy if it fades, gets covered, or DCF publishes a revision.',
    ],
    source: 'F.A.C. 65C-22.003(2)',
  },
  'fl.05.license_posted': {
    meaning:
      'Your current DCF license has to be posted in plain sight near the main entrance. An expired or missing license is one of the fastest paths to a citation.',
    steps: [
      'Frame the current license behind glass so it does not yellow.',
      'Hang it at adult eye-level inside the main entrance.',
      'When DCF mails the renewal, swap the new one in the same day.',
      'Use Levelbrook\'s license-expiration reminder to never miss a renewal.',
    ],
    source: 'F.A.C. 65C-22.001(7)',
  },
  'fl.07.staff_training': {
    meaning:
      'Every staff member has 12 months from their hire date to finish the DCF 45-hour Introductory Child Care Training. Missing this is one of the most common citations in Florida.',
    steps: [
      'Enroll new hires in the DCF online course in their first week.',
      'Track the 12-month deadline in Levelbrook\'s Staff Files.',
      'Save the completion certificate to the staff member\'s file.',
      'Run a quarterly audit so nobody slips past month 11.',
    ],
    source: 'F.A.C. 65C-22.003(4)',
  },
  'fl.09.cpr_first_aid': {
    meaning:
      'At least one staff member with current pediatric CPR + First Aid certification must be on-site every minute the facility is open — including during pickup and any field trips.',
    steps: [
      'Schedule shifts so every shift has a certified person on the floor.',
      'Stagger renewal dates so they never all expire in the same month.',
      'Keep the certificate cards in each staff file with expiration dates.',
      'Use Levelbrook\'s 90/60/30-day reminders to renew before expiry.',
    ],
    source: 'F.A.C. 65C-22.003(5)',
  },
  'fl.11.background_screening': {
    meaning:
      'Every owner, employee, and regular volunteer must have a current Level 2 background screening on file before working unsupervised. New hires can start under direct supervision while results are pending.',
    steps: [
      'Submit fingerprints through a DCF-approved Livescan vendor.',
      'Hold the new hire to supervised-only duty until the eligibility letter arrives.',
      'File the eligibility letter in the staff record.',
      'Re-screen every five years per Care Provider Background Screening Clearinghouse rules.',
    ],
    source: 'F.S. §435.04',
  },
  'fl.12.personnel_records': {
    meaning:
      'Each employee needs a complete CF-FSP 5337 Personnel Record — including the attestation of good moral character, the Background Screening Eligibility Letter, and proof of required training.',
    steps: [
      'Use the current CF-FSP 5337 form (not an older revision).',
      'Have every new hire complete it on their first day.',
      'Attach the screening letter and the training certificates.',
      'Store all files in a locked cabinet or a secure cloud folder.',
    ],
    source: 'F.A.C. 65C-22.006(4)',
  },
  'fl.13.child_enrollment_records': {
    meaning:
      'Every enrolled child must have a current CF-FSP 5219 enrollment form completed by the parent at admission, including emergency contacts and authorized pickup people.',
    steps: [
      'Have parents complete the form before the child\'s first day.',
      'Update the form annually and any time emergency contacts change.',
      'Keep the original or a clear copy in the child\'s file.',
      'Cross-check with your kiosk roster every 90 days.',
    ],
    source: 'F.A.C. 65C-22.006(2)',
  },
  'fl.14.immunization_records': {
    meaning:
      'Every enrolled child needs a current DH Form 680 (immunization record) or DH Form 681 (religious exemption) before they start care. Medical exemptions also go on Form 680.',
    steps: [
      'Collect the form before the child\'s first day — no exceptions.',
      'Mark renewal dates in Levelbrook so you chase parents before expiry.',
      'For exemptions, file the signed DH 681 and confirm it is the current revision.',
      'Re-confirm at the start of every school year.',
    ],
    source: 'F.S. §1003.22',
  },
  'fl.15.student_health_exam': {
    meaning:
      'Every enrolled child must have a current DH Form 3040 (student health examination) on file. The form is valid for 24 months from the exam date.',
    steps: [
      'Collect the form before the child\'s first day of care.',
      'Calendar the 24-month renewal in Levelbrook.',
      'Send the parent a reminder 60 and 30 days before expiry.',
      'Refuse care on the day after expiry if the new form has not arrived.',
    ],
    source: 'F.A.C. 65C-22.006(2)(c)',
  },
  'fl.24.hazardous_materials': {
    meaning:
      'Every cleaning product, medication, and other hazardous item must be in a locked cabinet that children cannot reach. Original labels must stay intact so any ingestion call to Poison Control can identify the product.',
    steps: [
      'Audit the entire facility for unlocked chemicals — diaper-changing tables included.',
      'Move everything into a cabinet with a child-resistant lock.',
      'Never decant chemicals into unlabeled spray bottles.',
      'Keep a Poison Control sticker (1-800-222-1222) on every classroom phone.',
    ],
    source: 'F.A.C. 65C-22.002(1)(d)',
  },
  'fl.25.fire_safety': {
    meaning:
      'You need a current annual fire inspection certificate, posted, with no open violations. Inspectors check the certificate first — then they ask whether the listed corrective actions were completed.',
    steps: [
      'Schedule the annual inspection 30 days before the current one expires.',
      'Close out any noted violations the same week.',
      'Post the certificate with the license, not buried in a binder.',
      'File the certificate in Levelbrook as a provider-level document.',
    ],
    source: 'F.A.C. 65C-22.002(4)',
  },
  'fl.27.emergency_drills': {
    meaning:
      'You must run a fire drill every month and a severe-weather (tornado) drill at least twice per year. Each drill needs a dated log entry.',
    steps: [
      'Log every drill in Levelbrook\'s Drills tab the day you run it.',
      'Snap a photo of the signed muster sheet and attach it to the drill.',
      'Vary the time of day across the year so staff practice realistic conditions.',
      'Review the log every quarter to make sure no month is missing.',
    ],
    source: 'F.A.C. 65C-22.002(7)',
  },
};

// Fallback copy used for any question that does not yet have a curated
// explainer. The phrasing avoids fear words — operators have enough of
// that already.
export const fallbackExplainer = {
  meaning:
    "We're still writing the plain-English explainer for this one.",
  steps: [
    'Check the rule citation below for the exact regulatory text.',
    'If you\'re not sure how to satisfy it, email levelbrookteam@gmail.com or call us during launch hours — we\'ll walk you through it.',
  ],
  source: '',
};

// hasExplainer is a convenience for the UI to render a slightly stronger
// affordance (the "Learn more" link) for items we have curated.
export function hasExplainer(id: string): boolean {
  return Object.prototype.hasOwnProperty.call(inspectionExplainers, id);
}

// getExplainer always returns something — the fallback if no curated
// entry exists. `source` is filled in with the item's own reference
// citation at the call site.
export function getExplainer(id: string): InspectionExplainer {
  return inspectionExplainers[id] ?? fallbackExplainer;
}
