// Atomic node-details RPC wrappers (ADR-015 §3).
//
// pluginId is passed explicitly on both calls, never inferred from the node id: node ids are
// plugin-chosen and two plugins may publish the same one under a connection.
import { getGateway } from '../backend/context';
import type { NodeDetails } from '../stores/nodeDetailsState';

export async function describeDiscoveryNode(
  connectionId: string,
  pluginId: string,
  nodeId: string
): Promise<NodeDetails | null> {
  const app = getGateway();
  if (!app || !app.DescribeDiscoveryNode) return null;
  const dto = await app.DescribeDiscoveryNode(connectionId, pluginId, nodeId);
  if (!dto) return null;
  return {
    sections: (dto.sections ?? []) as NodeDetails['sections'],
    values: dto.values ?? {},
    editable: !!dto.editable,
  };
}

/**
 * Saves edited values.
 *
 * The error escapes on purpose: a save can be refused by the host or by the plugin, and the panel
 * must show why rather than silently pretending it stuck.
 */
export async function applyDiscoveryNodeDetails(
  connectionId: string,
  pluginId: string,
  nodeId: string,
  values: Record<string, string>
): Promise<void> {
  const app = getGateway();
  if (!app || !app.ApplyDiscoveryNodeDetails) return;
  await app.ApplyDiscoveryNodeDetails(connectionId, pluginId, nodeId, values);
}
