export type RedisTreeItem = {
  id: string
  label: string
  depth: number
  isFolder: boolean
  isKey: boolean
  prefix: string
  childrenCount: number
}

type TreeNode = {
  label: string
  prefix: string
  children: Map<string, TreeNode>
  isKey: boolean
}

const splitKey = (key: string, separator: string, maxDepth: number): string[] => {
  if (!separator) return [key]
  const parts = key.split(separator).filter((part) => part.length > 0)
  if (maxDepth > 0 && parts.length > maxDepth) {
    const head = parts.slice(0, maxDepth - 1)
    const tail = parts.slice(maxDepth - 1).join(separator)
    return [...head, tail]
  }
  return parts
}

export const buildTree = (
  keys: string[],
  separator: string,
  maxDepth: number,
  expanded: Set<string>,
): RedisTreeItem[] => {
  const root: TreeNode = {
    label: '',
    prefix: '',
    children: new Map(),
    isKey: false,
  }

  for (const raw of keys) {
    if (raw === null || raw === undefined) continue
    const key = String(raw)
    const parts = splitKey(key, separator, maxDepth)
    if (!parts.length) continue
    let node = root
    const path: string[] = []
    parts.forEach((part, idx) => {
      path.push(part)
      const prefix = separator ? path.join(separator) : part
      let child = node.children.get(part)
      if (!child) {
        child = {
          label: part,
          prefix,
          children: new Map(),
          isKey: false,
        }
        node.children.set(part, child)
      }
      if (idx === parts.length - 1) {
        child.isKey = true
      }
      node = child
    })
  }

  const items: RedisTreeItem[] = []

  const walk = (node: TreeNode, depth: number) => {
    const children = Array.from(node.children.values())
    children.sort((a, b) => {
      const aFolder = a.children.size > 0
      const bFolder = b.children.size > 0
      if (aFolder !== bFolder) return aFolder ? -1 : 1
      return a.label.localeCompare(b.label)
    })
    for (const child of children) {
      const childrenCount = child.children.size
      const isFolder = childrenCount > 0
      items.push({
        id: child.prefix,
        label: child.label,
        depth,
        isFolder,
        isKey: child.isKey,
        prefix: child.prefix,
        childrenCount,
      })
      if (isFolder && expanded.has(child.prefix)) {
        walk(child, depth + 1)
      }
    }
  }

  walk(root, 0)
  return items
}
