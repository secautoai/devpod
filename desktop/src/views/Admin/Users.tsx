import {
  Badge,
  Box,
  Button,
  FormControl,
  FormLabel,
  HStack,
  Heading,
  IconButton,
  Input,
  Modal,
  ModalBody,
  ModalCloseButton,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ModalOverlay,
  Select,
  Table,
  Tbody,
  Td,
  Text,
  Th,
  Thead,
  Tr,
  VStack,
  useDisclosure,
  useToast,
} from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { useAuth } from "@/contexts"
import {
  adminListUsers,
  adminCreateUser,
  adminUpdateUser,
  adminDeleteUser,
  adminGetProviderPermissions,
  adminSetProviderPermissions,
  type ProviderPermissions,
} from "@/client/adminClient"
import type { UserPublic } from "@/client/authClient"

export function Users() {
  const { token } = useAuth()
  const toast = useToast()
  const createModal = useDisclosure()
  const permModal = useDisclosure()

  const [users, setUsers] = useState<UserPublic[]>([])
  const [loading, setLoading] = useState(true)
  const [selectedUser, setSelectedUser] = useState<UserPublic | null>(null)
  const [perms, setPerms] = useState<ProviderPermissions | null>(null)
  const [permInput, setPermInput] = useState("")

  // Create form state
  const [newUsername, setNewUsername] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [newRole, setNewRole] = useState<"admin" | "user">("user")
  const [creating, setCreating] = useState(false)

  const load = () => {
    if (!token) return
    adminListUsers(token)
      .then(setUsers)
      .catch((e) => toast({ status: "error", title: "Failed to load users", description: e.message }))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  const handleCreate = async () => {
    if (!token || !newUsername || !newPassword) return
    setCreating(true)
    try {
      await adminCreateUser(token, newUsername, newPassword, newRole)
      toast({ status: "success", title: `User "${newUsername}" created` })
      setNewUsername("")
      setNewPassword("")
      setNewRole("user")
      createModal.onClose()
      load()
    } catch (e: unknown) {
      toast({ status: "error", title: "Create failed", description: (e as Error).message })
    } finally {
      setCreating(false)
    }
  }

  const handleRoleToggle = async (u: UserPublic) => {
    if (!token) return
    const newRole = u.role === "admin" ? "user" : "admin"
    try {
      await adminUpdateUser(token, u.username, { role: newRole })
      toast({ status: "success", title: `Role updated to ${newRole}` })
      load()
    } catch (e: unknown) {
      toast({ status: "error", title: "Update failed", description: (e as Error).message })
    }
  }

  const handleDelete = async (u: UserPublic) => {
    if (!token) return
    if (!window.confirm(`Delete user "${u.username}"?`)) return
    try {
      await adminDeleteUser(token, u.username)
      toast({ status: "success", title: `User "${u.username}" deleted` })
      load()
    } catch (e: unknown) {
      toast({ status: "error", title: "Delete failed", description: (e as Error).message })
    }
  }

  const openPerms = async (u: UserPublic) => {
    if (!token) return
    setSelectedUser(u)
    try {
      const p = await adminGetProviderPermissions(token, u.username)
      setPerms(p)
      setPermInput(p.providers ? p.providers.join(", ") : "")
    } catch (e: unknown) {
      toast({ status: "error", title: "Load permissions failed", description: (e as Error).message })
      return
    }
    permModal.onOpen()
  }

  const savePerms = async () => {
    if (!token || !selectedUser) return
    const raw = permInput.trim()
    const providers: string[] | null =
      raw === "" ? null : raw.split(",").map((s) => s.trim()).filter(Boolean)
    try {
      await adminSetProviderPermissions(token, selectedUser.username, providers)
      toast({ status: "success", title: "Permissions updated" })
      permModal.onClose()
    } catch (e: unknown) {
      toast({ status: "error", title: "Save failed", description: (e as Error).message })
    }
  }

  return (
    <Box>
      <HStack justify="space-between" mb={4}>
        <Heading size="md">Users</Heading>
        <Button colorScheme="blue" size="sm" onClick={createModal.onOpen}>
          Add User
        </Button>
      </HStack>

      {loading ? (
        <Text>Loading…</Text>
      ) : (
        <Table size="sm" variant="simple">
          <Thead>
            <Tr>
              <Th>Username</Th>
              <Th>Role</Th>
              <Th>Created</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {users.map((u) => (
              <Tr key={u.id}>
                <Td fontFamily="mono">{u.username}</Td>
                <Td>
                  <Badge colorScheme={u.role === "admin" ? "purple" : "gray"}>{u.role}</Badge>
                </Td>
                <Td fontSize="xs">{new Date(u.createdAt).toLocaleDateString()}</Td>
                <Td>
                  <HStack spacing={1}>
                    <Button size="xs" variant="outline" onClick={() => handleRoleToggle(u)}>
                      {u.role === "admin" ? "Demote" : "Promote"}
                    </Button>
                    <Button size="xs" variant="outline" onClick={() => openPerms(u)}>
                      Providers
                    </Button>
                    <Button size="xs" colorScheme="red" variant="outline" onClick={() => handleDelete(u)}>
                      Delete
                    </Button>
                  </HStack>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      {/* Create User Modal */}
      <Modal isOpen={createModal.isOpen} onClose={createModal.onClose}>
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>Create User</ModalHeader>
          <ModalCloseButton />
          <ModalBody>
            <VStack spacing={3}>
              <FormControl isRequired>
                <FormLabel>Username</FormLabel>
                <Input
                  value={newUsername}
                  onChange={(e) => setNewUsername(e.target.value)}
                  placeholder="username"
                />
              </FormControl>
              <FormControl isRequired>
                <FormLabel>Password</FormLabel>
                <Input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="••••••••"
                />
              </FormControl>
              <FormControl>
                <FormLabel>Role</FormLabel>
                <Select value={newRole} onChange={(e) => setNewRole(e.target.value as "admin" | "user")}>
                  <option value="user">User</option>
                  <option value="admin">Admin</option>
                </Select>
              </FormControl>
            </VStack>
          </ModalBody>
          <ModalFooter>
            <Button variant="ghost" mr={3} onClick={createModal.onClose}>
              Cancel
            </Button>
            <Button colorScheme="blue" isLoading={creating} onClick={handleCreate}>
              Create
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Provider Permissions Modal */}
      <Modal isOpen={permModal.isOpen} onClose={permModal.onClose}>
        <ModalOverlay />
        <ModalContent>
          <ModalHeader>Provider Permissions — {selectedUser?.username}</ModalHeader>
          <ModalCloseButton />
          <ModalBody>
            <VStack spacing={3} align="start">
              <Text fontSize="sm" color="gray.500">
                Comma-separated provider IDs (e.g. <code>docker, kubernetes</code>).
                Leave empty for unrestricted access.
              </Text>
              <FormControl>
                <FormLabel>Allowed Providers</FormLabel>
                <Input
                  value={permInput}
                  onChange={(e) => setPermInput(e.target.value)}
                  placeholder="docker, kubernetes (empty = unrestricted)"
                />
              </FormControl>
              {perms && !perms.unrestricted && (
                <Badge colorScheme="orange">Restricted</Badge>
              )}
              {perms?.unrestricted && (
                <Badge colorScheme="green">Unrestricted</Badge>
              )}
            </VStack>
          </ModalBody>
          <ModalFooter>
            <Button variant="ghost" mr={3} onClick={permModal.onClose}>
              Cancel
            </Button>
            <Button colorScheme="blue" onClick={savePerms}>
              Save
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Box>
  )
}
