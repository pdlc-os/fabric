## Fan out sample

Initialize this example as a grove:

```bash
fabric init
```

Create a researcher template

```bash
fabric templates clone default researcher
```

Replace the system instruction in the template with the researcher prompt content 

```bash
mv ./researcher-prompt.md $(fabric config dir)/templates/researcher/system-prompt.md
```

Start the workstation server

```bash
fabric server start
```

enable hub and link grove

```bash
fabric config set hub.endpoint http://localhost:8080
fabric hub enable
fabric hub link
```

This will prompt you to sync templates.

Then create and start an orchestrator agent:


```bash
fabric start -a orchestrator
```


Then in an orchestrator agent use the following prompt

```
Use the fabric cli tool to start a researcher agent for each of the topics in topics.txt

Be sure to ask to be --notified when they are done

Once you have started each researcher wait idle for you to be notified, no need to poll or check on them, just wait for their notification.

When a research completes their work, you may delete them
```

