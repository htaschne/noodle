# Roteiro de apresentação

Material para uma apresentação de aproximadamente 5 minutos. Os seis slides correspondem ao roteiro abaixo. Informar os nomes reais dos integrantes na apresentação e gravar a demonstração antes de entregar o link do vídeo. O relatório individual deve descrever as experiências reais de cada integrante.

## 1. Noodle

O Noodle envia lembretes pessoais de tarefas do Moodle que estão próximas do prazo e ainda não foram enviadas. O objetivo é apoiar o estudante na organização das pendências. O projeto aplica integração REST, comunicação assíncrona e execução em contêineres. Não houve medição de impacto sobre participação ou evasão.

## 2. Autorização por estudante

Cada estudante configura sua própria conta Moodle e um e-mail destinatário. O serviço consulta apenas as tarefas e o estado de entrega dessa conta. Não depende de acesso aos colegas. Nesta versão, cada worker usa uma configuração; um cadastro compartilhado de múltiplas contas não foi implementado.

## 3. Arquitetura

O microsserviço de Acompanhamento, escrito em Go, consulta o Moodle periodicamente. Ele publica uma solicitação na fila RabbitMQ quando encontra trabalho elegível. O microsserviço de Notificações, em Java, consome essa solicitação, envia o e-mail por SMTP e registra o identificador no SQLite. O Mailpit captura as mensagens nos testes. As duas aplicações podem ser construídas e executadas separadamente.

Referência: [Arquitetura](ARCHITECTURE.md).

## 4. Regras do lembrete

A tarefa precisa ser individual, permitir envios e vencer depois do momento atual, dentro das próximas 24 horas. A aplicação consulta a última tentativa do estudante autenticado. Estados `new`, `draft` e `reopened` indicam pendência. Se não existir registro de envio, a tentativa explícita precisa confirmar permissão de edição, envio habilitado e ausência de bloqueio ou avaliação. Trabalho enviado, vencido, em grupo, com prorrogação individual ou com estado incerto é excluído.

O identificador combina curso, tarefa e estudante. O histórico persistente evita repetir uma entrega concluída durante o reprocessamento normal e após reinícios. Há uma janela de falha entre o envio SMTP e a gravação do histórico, descrita na documentação.

## 5. Demonstração dos critérios

Executar, na raiz do repositório:

```sh
python3 tests/acceptance/run.py
```

Mostrar as quatro linhas `PASS criterion` e explicar o que foi observado:

1. Uma tarefa ainda não iniciada, com prazo em uma hora, produz um e-mail com nome, prazo e link.
2. As tarefas já enviadas, fora da janela, vencidas ou em grupo não produzem e-mails adicionais.
3. O reprocessamento após reiniciar o consumidor e recriar o cliente de monitoramento não duplica a entrega.
4. O Mailpit rejeita temporariamente o SMTP com código 451. A falha é registrada e a nova tentativa entrega o lembrete quando a rejeição é removida.

O teste utiliza respostas Moodle fictícias, RabbitMQ real, a aplicação real de Notificações e SMTP do Mailpit. O teste termina com dois e-mails capturados para um endereço fictício e remove seu próprio ambiente. Não exibir o arquivo `.env` durante a gravação.

Referências: [resultados e reprodução](../tests/acceptance/README.md) e [transcrição registrada](../tests/acceptance/last-run.log).

## 6. Entrega e limites

O repositório inclui código, instalação, arquitetura e testes reproduzíveis. A API pessoal também foi validada por chamadas de leitura ao Moodle da PUCRS, sem publicação de notificações. Essa verificação de compatibilidade é distinta da demonstração controlada do fluxo completo.

O calendário iCalendar continua disponível como fonte alternativa, mas não informa se o estudante já enviou a tarefa. Atividades ocultas ou permissões da instituição também podem limitar a cobertura. Para lembretes que excluem tarefas enviadas, usar `MOODLE_SOURCE=api`.

Antes da entrega acadêmica, publicar o repositório conforme solicitado pela disciplina, revisar os nomes dos integrantes e enviar o link do vídeo. O roteiro não substitui a gravação nem o relatório individual.
