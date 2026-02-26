import {
  Badge,
  Box,
  Button,
  Code,
  HStack,
  Heading,
  Input,
  Select,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tooltip,
  Tr,
  useToast,
} from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { useAuth } from "@/contexts"
import { adminListLogs, type OperationLog } from "@/client/adminClient"

const PAGE_SIZE = 20

export function OperationLogs() {
  const { token } = useAuth()
  const toast = useToast()
  const [logs, setLogs] = useState<OperationLog[]>([])
  const [loading, setLoading] = useState(true)
  const [offset, setOffset] = useState(0)
  const [filterUsername, setFilterUsername] = useState("")
  const [filterAction, setFilterAction] = useState("")

  const load = (off = offset) => {
    if (!token) return
    setLoading(true)
    adminListLogs(token, {
      username: filterUsername || undefined,
      action: filterAction || undefined,
      limit: PAGE_SIZE,
      offset: off,
    })
      .then(setLogs)
      .catch((e) =>
        toast({ status: "error", title: "Failed to load logs", description: e.message })
      )
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load(0)
    setOffset(0)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token, filterUsername, filterAction])

  const prev = () => {
    const next = Math.max(0, offset - PAGE_SIZE)
    setOffset(next)
    load(next)
  }
  const next = () => {
    const n = offset + PAGE_SIZE
    setOffset(n)
    load(n)
  }

  return (
    <Box>
      <HStack justify="space-between" mb={4}>
        <Heading size="md">Operation Logs</Heading>
      </HStack>

      <HStack spacing={3} mb={4}>
        <Input
          placeholder="Filter by username"
          size="sm"
          value={filterUsername}
          onChange={(e) => setFilterUsername(e.target.value)}
          maxW="180px"
        />
        <Select
          size="sm"
          maxW="160px"
          value={filterAction}
          onChange={(e) => setFilterAction(e.target.value)}>
          <option value="">All actions</option>
          <option value="create">create</option>
          <option value="delete">delete</option>
          <option value="start">start</option>
          <option value="stop">stop</option>
          <option value="status">status</option>
          <option value="list">list</option>
          <option value="up">up</option>
        </Select>
        <Button size="sm" onClick={() => load(offset)}>
          Refresh
        </Button>
      </HStack>

      {loading ? (
        <Text>Loading…</Text>
      ) : (
        <>
          <Table size="sm" variant="simple">
            <Thead>
              <Tr>
                <Th>Time</Th>
                <Th>User</Th>
                <Th>Action</Th>
                <Th>Target</Th>
                <Th>Status</Th>
              </Tr>
            </Thead>
            <Tbody>
              {logs.length === 0 && (
                <Tr>
                  <Td colSpan={5} textAlign="center" color="gray.400">
                    No logs found
                  </Td>
                </Tr>
              )}
              {logs.map((log) => (
                <Tr key={log.id}>
                  <Td fontSize="xs" whiteSpace="nowrap">
                    {new Date(log.startedAt).toLocaleString()}
                  </Td>
                  <Td fontFamily="mono" fontSize="sm">
                    {log.username || "—"}
                  </Td>
                  <Td>
                    <Code fontSize="xs">{log.action}</Code>
                  </Td>
                  <Td fontFamily="mono" fontSize="sm">
                    {log.target || "—"}
                  </Td>
                  <Td>
                    {log.finishedAt ? (
                      <Tooltip label={log.error || undefined} isDisabled={!log.error}>
                        <Badge colorScheme={log.exitCode === 0 ? "green" : "red"}>
                          {log.exitCode === 0 ? "ok" : `exit ${log.exitCode}`}
                        </Badge>
                      </Tooltip>
                    ) : (
                      <Badge colorScheme="blue">running</Badge>
                    )}
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>

          <HStack mt={3} justify="end" spacing={3}>
            <Button size="xs" onClick={prev} isDisabled={offset === 0}>
              ← Prev
            </Button>
            <Text fontSize="xs">offset {offset}</Text>
            <Button size="xs" onClick={next} isDisabled={logs.length < PAGE_SIZE}>
              Next →
            </Button>
          </HStack>
        </>
      )}
    </Box>
  )
}
