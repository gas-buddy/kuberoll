status gets current deployment revision
if Generation <= status.observedGeneration
  get deployment condition
  if condition and reason = util.timedout, "deployment exceeded it's progress deadline"
  if spec.replicas != nil and status.updatedreplicas < spec.replicas, "updatedReplicas out of replicas updated"
  if status.replicas > status.updatedReplicas "status.replicas-status.updatedreplicas pending termination"
  if status.availablereplicas < status.updatedreplicas, "status.availablereplicas of status.updated replicas available"
  successful


getconditions:
  range on status.conditions, check .Type

revision:
  meta.Accessor(thing)
  thing.GetAnnoitations()[deployment.kubernetes.io/revision]
