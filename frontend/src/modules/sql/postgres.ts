export function getPostgresHint() {
  return 'Required: name, host, port (default 5432), username. Optional: database (defaults to postgres), options.sslmode.'
}

const postgresKeywordsNeedingQuotes = new Set(
  `
all
analyse
analyze
and
any
array
as
asc
asymmetric
authorization
between
binary
both
case
cast
check
collate
column
constraint
create
current_catalog
current_date
current_role
current_schema
current_time
current_timestamp
current_user
default
deferrable
desc
distinct
do
else
end
except
false
for
foreign
from
grant
group
having
in
initially
intersect
into
leading
limit
localtime
localtimestamp
not
null
offset
on
only
or
order
placing
primary
references
returning
select
session_user
some
symmetric
table
then
to
trailing
true
union
unique
user
using
variadic
when
where
window
with
`.trim().split(/\s+/),
)

const escapePostgresIdentifier = (value: string) => value.replaceAll('"', '""')
const isSimplePostgresIdentifier = (value: string) => /^[a-z_][a-z0-9_$]*$/.test(value)

const shouldQuotePostgresIdentifier = (value: string) => {
  if (!isSimplePostgresIdentifier(value)) return true
  return postgresKeywordsNeedingQuotes.has(value.toLowerCase())
}

const splitPostgresIdentifierPath = (value: string) => {
  const out: string[] = []
  let current = ''
  let inQuotedIdentifier = false

  for (let i = 0; i < value.length; i += 1) {
    const ch = value[i]
    if (ch === '"') {
      current += ch
      if (inQuotedIdentifier) {
        const next = value[i + 1]
        if (next === '"') {
          current += next
          i += 1
        } else {
          inQuotedIdentifier = false
        }
      } else {
        inQuotedIdentifier = true
      }
      continue
    }
    if (ch === '.' && !inQuotedIdentifier) {
      out.push(current)
      current = ''
      continue
    }
    current += ch
  }

  out.push(current)
  return out
}

type PostgresIdentifierQuoteOptions = {
  treatDotAsPath?: boolean
}

export const quotePostgresIdentifierIfNeeded = (
  value: string,
  options: PostgresIdentifierQuoteOptions = {},
) => {
  const trimmed = String(value || '').trim()
  if (!trimmed) return trimmed

  const treatDotAsPath = options.treatDotAsPath !== false
  if (!treatDotAsPath) {
    if (trimmed === '*') return trimmed
    if (trimmed.startsWith('"') && trimmed.endsWith('"') && trimmed.length >= 2) return trimmed
    if (!shouldQuotePostgresIdentifier(trimmed)) return trimmed
    return `"${escapePostgresIdentifier(trimmed)}"`
  }

  return splitPostgresIdentifierPath(trimmed)
    .map((part) => {
      const segment = part.trim()
      if (!segment || segment === '*') return segment
      if (segment.startsWith('"') && segment.endsWith('"') && segment.length >= 2) return segment
      if (!shouldQuotePostgresIdentifier(segment)) return segment
      return `"${escapePostgresIdentifier(segment)}"`
    })
    .join('.')
}
