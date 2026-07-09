

Orchestration Prompt:

Your job is to orchestrate a set of Agents across a sequence of work

For each of the tasks in @worksequence use the fabric cli tool to start an agent and assign it an item of work, being sure to include the --notify argument so it will alert you when it has completed its task

after assigning the task, wait in idle for the agent's notification.

stop and delete that agent and then proceed to assign the next item of work to a new agent

 when all agents have completed their work you are done
