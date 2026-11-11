import {
  Box,
  Flex,
  HStack,
  Link,
  Tab,
  TabList,
  TabPanel,
  TabPanels,
  Tabs,
  Text,
  useColorModeValue,
} from "@chakra-ui/react"
import { Link as RouterLink, useMatch } from "react-router-dom"
import { Routes } from "@/routes.constants"
import { Users } from "./Users"
import { OperationLogs } from "./OperationLogs"
import { BrandingSettingsPanel } from "./BrandingSettings"

export function AdminLayout() {
  const usersMatch = useMatch(Routes.ADMIN_USERS)
  const logsMatch = useMatch(Routes.ADMIN_LOGS)
  const brandingMatch = useMatch(Routes.ADMIN_BRANDING)

  const defaultIndex = brandingMatch ? 2 : logsMatch ? 1 : 0

  return (
    <Box>
      <HStack mb={6} spacing={2}>
        <Text fontWeight="bold" fontSize="xl">
          Admin
        </Text>
      </HStack>

      <Tabs defaultIndex={defaultIndex} isLazy variant="enclosed">
        <TabList>
          <Tab as={RouterLink} to={Routes.ADMIN_USERS}>
            Users
          </Tab>
          <Tab as={RouterLink} to={Routes.ADMIN_LOGS}>
            Operation Logs
          </Tab>
          <Tab as={RouterLink} to={Routes.ADMIN_BRANDING}>
            Branding
          </Tab>
        </TabList>
        <TabPanels>
          <TabPanel>
            <Users />
          </TabPanel>
          <TabPanel>
            <OperationLogs />
          </TabPanel>
          <TabPanel>
            <BrandingSettingsPanel />
          </TabPanel>
        </TabPanels>
      </Tabs>
    </Box>
  )
}
